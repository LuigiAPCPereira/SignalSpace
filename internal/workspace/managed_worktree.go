package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const managedMetadataVersion = 1

type ManagedWorkspaceState string

const (
	ManagedWorkspaceAvailable    ManagedWorkspaceState = "available"
	ManagedWorkspaceActive       ManagedWorkspaceState = "active"
	ManagedWorkspaceDirty        ManagedWorkspaceState = "dirty"
	ManagedWorkspaceMissing      ManagedWorkspaceState = "missing"
	ManagedWorkspaceStale        ManagedWorkspaceState = "stale"
	ManagedWorkspaceInconsistent ManagedWorkspaceState = "inconsistent"
	managedWorkspaceCreating     ManagedWorkspaceState = "creating"
)

var (
	ErrManagedWorkspaceNotFound     = errors.New("managed workspace not found")
	ErrManagedWorkspaceMissing      = errors.New("managed workspace is missing")
	ErrManagedWorkspaceDirty        = errors.New("managed workspace is dirty")
	ErrManagedWorkspaceActive       = errors.New("managed workspace is active")
	ErrManagedWorkspaceStale        = errors.New("managed workspace source is stale")
	ErrManagedWorkspaceInconsistent = errors.New("managed workspace metadata is inconsistent")
	ErrManagedWorkspaceBusy         = errors.New("managed workspace manager is busy")
	ErrManagedWorkspaceClosed       = errors.New("managed workspace manager is closed")
	ErrInvalidManagedSource         = errors.New("invalid managed Git source")
	ErrInvalidManagedBase           = errors.New("invalid managed Git base")
	ErrUnsupportedWorktreeFilter    = errors.New("managed worktree Git filters are unsupported")
	ErrManagedMetadata              = errors.New("managed workspace metadata is invalid")
)

// ManagedWorkspaceDescriptor é seguro para exibição e transporte: não inclui
// raiz do checkout, raiz de origem, .git ou qualquer caminho absoluto.
type ManagedWorkspaceDescriptor struct {
	WorkspaceID string                `json:"workspace_id"`
	Mode        string                `json:"mode"`
	State       ManagedWorkspaceState `json:"state"`
	Active      bool                  `json:"active"`
	BaseRef     string                `json:"base_ref"`
	BaseSHA     string                `json:"base_sha"`
	DirtySource bool                  `json:"dirty_source"`
}

type managedWorkspaceMetadata struct {
	Version     int                   `json:"version"`
	WorkspaceID string                `json:"workspace_id"`
	Mode        string                `json:"mode"`
	SourceRoot  string                `json:"source_root"`
	ManagedRoot string                `json:"managed_root"`
	BaseRef     string                `json:"base_ref"`
	BaseSHA     string                `json:"base_sha"`
	DirtySource bool                  `json:"dirty_source"`
	State       ManagedWorkspaceState `json:"state"`
	CreatedAt   time.Time             `json:"created_at"`
}

// ManagedWorktreeManager guarda apenas o lifecycle local. Ele não é injetado
// nos handlers MCP e não cria uma segunda sessão concorrente em Grants.
type ManagedWorktreeManager struct {
	mu                   sync.Mutex
	stateDir             string
	worktreesDir         string
	hooksDir             string
	records              map[string]*managedWorkspaceMetadata
	inconsistentMetadata map[string]error
	activeID             string
	closed               bool
	runner               managedGitRunner
}

func NewManagedWorktreeManager(stateDir string) (*ManagedWorktreeManager, error) {
	if !validManagedStateDir(stateDir) {
		return nil, ErrInvalidRoot
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "worktrees"), 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "hooks"), 0o700); err != nil {
		return nil, err
	}
	for _, directory := range []string{stateDir, filepath.Join(stateDir, "worktrees"), filepath.Join(stateDir, "hooks")} {
		if err := os.Chmod(directory, 0o700); err != nil {
			return nil, err
		}
		if resolved, err := filepath.EvalSymlinks(directory); err != nil || resolved != directory {
			return nil, ErrUnsafePath
		}
	}
	m := &ManagedWorktreeManager{
		stateDir:             stateDir,
		worktreesDir:         filepath.Join(stateDir, "worktrees"),
		hooksDir:             filepath.Join(stateDir, "hooks"),
		records:              make(map[string]*managedWorkspaceMetadata),
		inconsistentMetadata: make(map[string]error),
		runner:               managedGitRunner{hooksPath: filepath.Join(stateDir, "hooks")},
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func validManagedStateDir(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && path != "/" && !strings.ContainsRune(path, '\x00')
}

// ValidateCreateRequest é usado pelo console para validar antes da confirmação.
// A validação não cria worktree, não altera a origem e não resolve base remota.
func (m *ManagedWorktreeManager) ValidateCreateRequest(sourceRoot, baseRef string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrManagedWorkspaceClosed
	}
	_, _, _, _, err := m.inspectCreateSource(sourceRoot, baseRef)
	return err
}

func (m *ManagedWorktreeManager) Create(sourceRoot, baseRef string) (ManagedWorkspaceDescriptor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ManagedWorkspaceDescriptor{}, ErrManagedWorkspaceClosed
	}
	if m.activeID != "" {
		return ManagedWorkspaceDescriptor{}, ErrManagedWorkspaceBusy
	}
	source, resolvedRef, baseSHA, dirtySource, err := m.inspectCreateSource(sourceRoot, baseRef)
	if err != nil {
		return ManagedWorkspaceDescriptor{}, err
	}
	if filepathHasPrefix(source, m.worktreesDir) {
		return ManagedWorkspaceDescriptor{}, ErrInvalidManagedSource
	}
	workspaceID, err := newManagedWorkspaceID()
	if err != nil {
		return ManagedWorkspaceDescriptor{}, err
	}
	managedRoot := filepath.Join(m.worktreesDir, workspaceID)
	record := &managedWorkspaceMetadata{
		Version: managedMetadataVersion, WorkspaceID: workspaceID, Mode: WorkspaceModeWorktree,
		SourceRoot: source, ManagedRoot: managedRoot, BaseRef: resolvedRef, BaseSHA: baseSHA,
		DirtySource: dirtySource, State: managedWorkspaceCreating, CreatedAt: time.Now().UTC(),
	}
	m.records[workspaceID] = record
	if err := m.writeMetadata(record); err != nil {
		delete(m.records, workspaceID)
		return ManagedWorkspaceDescriptor{}, err
	}
	created := false
	rollback := func(operationErr error) error {
		if !created {
			delete(m.records, workspaceID)
			_ = os.Remove(m.metadataPath(workspaceID))
			return operationErr
		}
		if err := m.runner.run(source, "worktree", "remove", "--force", managedRoot); err != nil {
			record.State = ManagedWorkspaceInconsistent
			_ = m.writeMetadata(record)
			return errors.Join(operationErr, ErrManagedWorkspaceInconsistent, err)
		}
		if _, err := os.Lstat(managedRoot); err == nil {
			record.State = ManagedWorkspaceInconsistent
			_ = m.writeMetadata(record)
			return errors.Join(operationErr, ErrManagedWorkspaceInconsistent)
		}
		delete(m.records, workspaceID)
		_ = os.Remove(m.metadataPath(workspaceID))
		return operationErr
	}
	if err := m.runner.run(source, "worktree", "add", "--detach", "--no-checkout", managedRoot, baseSHA); err != nil {
		if _, statErr := os.Lstat(managedRoot); statErr == nil {
			created = true
		}
		return ManagedWorkspaceDescriptor{}, rollback(err)
	}
	created = true
	if err := m.validateNoUnsupportedFeatures(source); err != nil {
		return ManagedWorkspaceDescriptor{}, rollback(err)
	}
	if err := m.runner.run(managedRoot, "checkout", "--detach", "--force", baseSHA); err != nil {
		return ManagedWorkspaceDescriptor{}, rollback(err)
	}
	if err := m.validateManagedAssociation(record); err != nil {
		return ManagedWorkspaceDescriptor{}, rollback(err)
	}
	record.State = ManagedWorkspaceAvailable
	if err := m.writeMetadata(record); err != nil {
		return ManagedWorkspaceDescriptor{}, rollback(err)
	}
	return m.descriptorLocked(record), nil
}

func (m *ManagedWorktreeManager) Activate(workspaceID, clientID string, grants *Grants, scopes ...string) (string, ManagedWorkspaceDescriptor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", ManagedWorkspaceDescriptor{}, ErrManagedWorkspaceClosed
	}
	if grants == nil || !validWorkspaceID(workspaceID) {
		return "", ManagedWorkspaceDescriptor{}, ErrManagedWorkspaceNotFound
	}
	record, ok := m.records[workspaceID]
	if !ok {
		return "", ManagedWorkspaceDescriptor{}, ErrManagedWorkspaceNotFound
	}
	if err := m.refreshLocked(record); err != nil {
		return "", ManagedWorkspaceDescriptor{}, err
	}
	if record.State != ManagedWorkspaceAvailable && record.State != ManagedWorkspaceDirty {
		return "", ManagedWorkspaceDescriptor{}, managedStateError(record.State)
	}
	if m.activeID != "" && m.activeID != workspaceID {
		return "", ManagedWorkspaceDescriptor{}, ErrManagedWorkspaceBusy
	}
	canonical, err := NormalizeManagedCapabilities(scopes...)
	if err != nil {
		return "", ManagedWorkspaceDescriptor{}, err
	}
	metadata := WorkspaceMetadata{Mode: WorkspaceModeWorktree, ManagedWorkspaceID: record.WorkspaceID, BaseRef: record.BaseRef, BaseSHA: record.BaseSHA, DirtySource: record.DirtySource}
	sessionID, err := grants.GrantManagedWithScopes(record.ManagedRoot, clientID, metadata, canonical...)
	if err != nil {
		return "", ManagedWorkspaceDescriptor{}, err
	}
	m.activeID = workspaceID
	return sessionID, m.descriptorLocked(record), nil
}

func (m *ManagedWorktreeManager) Deactivate(workspaceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrManagedWorkspaceClosed
	}
	if workspaceID == "" || m.activeID != workspaceID {
		return ErrManagedWorkspaceNotFound
	}
	m.activeID = ""
	if record := m.records[workspaceID]; record != nil {
		_ = m.refreshLocked(record)
	}
	return nil
}

func (m *ManagedWorktreeManager) ActiveID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeID
}

func (m *ManagedWorktreeManager) List() ([]ManagedWorkspaceDescriptor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrManagedWorkspaceClosed
	}
	result := make([]ManagedWorkspaceDescriptor, 0, len(m.records))
	for _, record := range m.records {
		if err := m.refreshLocked(record); err != nil && !errors.Is(err, ErrManagedWorkspaceMissing) && !errors.Is(err, ErrManagedWorkspaceStale) {
			record.State = ManagedWorkspaceInconsistent
		}
		result = append(result, m.descriptorLocked(record))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].WorkspaceID < result[j].WorkspaceID })
	return result, nil
}

func (m *ManagedWorktreeManager) Descriptor(workspaceID string) (ManagedWorkspaceDescriptor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.records[workspaceID]
	if !ok {
		return ManagedWorkspaceDescriptor{}, ErrManagedWorkspaceNotFound
	}
	if err := m.refreshLocked(record); err != nil && !errors.Is(err, ErrManagedWorkspaceMissing) && !errors.Is(err, ErrManagedWorkspaceStale) {
		record.State = ManagedWorkspaceInconsistent
	}
	return m.descriptorLocked(record), nil
}

func (m *ManagedWorktreeManager) Remove(workspaceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrManagedWorkspaceClosed
	}
	if m.activeID == workspaceID {
		return ErrManagedWorkspaceActive
	}
	record, ok := m.records[workspaceID]
	if !ok {
		return ErrManagedWorkspaceNotFound
	}
	if err := m.refreshLocked(record); err != nil {
		return err
	}
	if record.State == ManagedWorkspaceDirty {
		return ErrManagedWorkspaceDirty
	}
	if record.State != ManagedWorkspaceAvailable {
		return managedStateError(record.State)
	}
	if err := m.validateManagedAssociation(record); err != nil {
		return errors.Join(ErrManagedWorkspaceInconsistent, err)
	}
	if err := m.runner.run(record.SourceRoot, "worktree", "remove", record.ManagedRoot); err != nil {
		return err
	}
	if _, err := os.Lstat(record.ManagedRoot); err == nil {
		return ErrManagedWorkspaceInconsistent
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(m.metadataPath(workspaceID)); err != nil {
		return errors.Join(ErrManagedWorkspaceInconsistent, err)
	}
	delete(m.records, workspaceID)
	return nil
}

func (m *ManagedWorktreeManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	return nil
}

func (m *ManagedWorktreeManager) load() error {
	entries, err := os.ReadDir(m.worktreesDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(m.worktreesDir, entry.Name())
		data, readErr := readLimitedPrivate(path, 64<<10)
		if readErr != nil {
			m.inconsistentMetadata[path] = readErr
			continue
		}
		var record managedWorkspaceMetadata
		decoder := json.NewDecoder(strings.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil || !validManagedMetadata(record, m.worktreesDir) {
			m.inconsistentMetadata[path] = ErrManagedMetadata
			continue
		}
		m.records[record.WorkspaceID] = &record
	}
	for _, record := range m.records {
		_ = m.refreshLocked(record)
	}
	return nil
}

func validManagedMetadata(record managedWorkspaceMetadata, worktreesDir string) bool {
	return record.Version == managedMetadataVersion && record.Mode == WorkspaceModeWorktree &&
		validWorkspaceID(record.WorkspaceID) && filepath.Clean(record.ManagedRoot) == filepath.Join(worktreesDir, record.WorkspaceID) &&
		filepath.IsAbs(record.SourceRoot) && filepath.Clean(record.SourceRoot) == record.SourceRoot &&
		validManagedRef(record.BaseRef) && validObjectID(record.BaseSHA) && validManagedState(record.State)
}

func (m *ManagedWorktreeManager) inspectCreateSource(sourceRoot, baseRef string) (string, string, string, bool, error) {
	if err := ValidateApprovedRoot(sourceRoot); err != nil {
		return "", "", "", false, errors.Join(ErrInvalidManagedSource, err)
	}
	if resolved, err := filepath.EvalSymlinks(sourceRoot); err != nil || resolved != sourceRoot {
		return "", "", "", false, ErrInvalidManagedSource
	}
	gitInfo, err := os.Lstat(filepath.Join(sourceRoot, ".git"))
	if err != nil || !gitInfo.IsDir() {
		return "", "", "", false, ErrInvalidManagedSource
	}
	if filepathHasPrefix(sourceRoot, m.worktreesDir) {
		return "", "", "", false, ErrInvalidManagedSource
	}
	if filepathHasPrefix(sourceRoot, m.stateDir) {
		return "", "", "", false, ErrInvalidManagedSource
	}
	if output, err := m.runner.capture(sourceRoot, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(output) != "true" {
		return "", "", "", false, ErrInvalidManagedSource
	}
	if output, err := m.runner.capture(sourceRoot, "rev-parse", "--is-bare-repository"); err != nil || strings.TrimSpace(output) != "false" {
		return "", "", "", false, ErrInvalidManagedSource
	}
	if output, err := m.runner.capture(sourceRoot, "rev-parse", "--show-toplevel"); err != nil || filepath.Clean(strings.TrimSpace(output)) != sourceRoot {
		return "", "", "", false, ErrInvalidManagedSource
	}
	if err := m.validateNoUnsupportedFeatures(sourceRoot); err != nil {
		return "", "", "", false, err
	}
	ref := baseRef
	if ref == "" {
		ref = "HEAD"
	}
	if !validManagedRef(ref) {
		return "", "", "", false, ErrInvalidManagedBase
	}
	output, err := m.runner.capture(sourceRoot, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", "", "", false, ErrInvalidManagedBase
	}
	baseSHA := strings.TrimSpace(output)
	if !validObjectID(baseSHA) {
		return "", "", "", false, ErrInvalidManagedBase
	}
	status, err := m.runner.capture(sourceRoot, "status", "--porcelain=v1", "--untracked-files=all", "--")
	if err != nil {
		return "", "", "", false, err
	}
	return sourceRoot, ref, baseSHA, strings.TrimSpace(status) != "", nil
}

func (m *ManagedWorktreeManager) validateNoUnsupportedFeatures(sourceRoot string) error {
	output, err := m.runner.capture(sourceRoot, "config", "--local", "--get-regexp", `^filter\..*\.(clean|smudge|process)$`)
	if err != nil {
		if code, ok := managedGitExitCode(err); ok && code == 1 && strings.TrimSpace(output) == "" {
			// git config uses exit 1 for no matching keys.
		} else {
			return ErrManagedGit
		}
	}
	if strings.TrimSpace(output) != "" {
		return ErrUnsupportedWorktreeFilter
	}
	output, err = m.runner.capture(sourceRoot, "ls-files", "--stage", "--")
	if err != nil {
		return ErrManagedGit
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "160000 ") {
			return ErrUnsupportedWorktreeFilter
		}
	}
	return nil
}

func (m *ManagedWorktreeManager) validateManagedAssociation(record *managedWorkspaceMetadata) error {
	if _, err := os.Stat(record.ManagedRoot); err != nil {
		return errors.Join(ErrManagedWorkspaceMissing, err)
	}
	if resolved, err := filepath.EvalSymlinks(record.SourceRoot); err != nil || resolved != record.SourceRoot {
		return ErrManagedWorkspaceStale
	}
	if sourceTop, err := m.runner.capture(record.SourceRoot, "rev-parse", "--show-toplevel"); err != nil || filepath.Clean(strings.TrimSpace(sourceTop)) != record.SourceRoot {
		return ErrManagedWorkspaceStale
	}
	if output, err := m.runner.capture(record.ManagedRoot, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(output) != "true" {
		return ErrManagedWorkspaceInconsistent
	}
	sourceCommon, err := m.gitCommonDir(record.SourceRoot)
	if err != nil {
		return ErrManagedWorkspaceStale
	}
	managedCommon, err := m.gitCommonDir(record.ManagedRoot)
	if err != nil || sourceCommon != managedCommon {
		return ErrManagedWorkspaceInconsistent
	}
	if output, err := m.runner.capture(record.ManagedRoot, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil || strings.TrimSpace(output) != "" {
		return ErrManagedWorkspaceInconsistent
	} else if code, ok := managedGitExitCode(err); !ok || code != 1 {
		return ErrManagedWorkspaceInconsistent
	}
	if output, err := m.runner.capture(record.ManagedRoot, "rev-parse", "HEAD"); err != nil || strings.TrimSpace(output) != record.BaseSHA {
		return ErrManagedWorkspaceInconsistent
	}
	if err := m.validateNoUnsupportedFeatures(record.ManagedRoot); err != nil {
		return err
	}
	return nil
}

func (m *ManagedWorktreeManager) gitCommonDir(directory string) (string, error) {
	output, err := m.runner.capture(directory, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	common := strings.TrimSpace(output)
	if common == "" || !filepath.IsAbs(common) {
		common = filepath.Join(directory, common)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(common))
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func (m *ManagedWorktreeManager) refreshLocked(record *managedWorkspaceMetadata) error {
	if record.State == ManagedWorkspaceInconsistent {
		return ErrManagedWorkspaceInconsistent
	}
	if _, err := os.Lstat(record.ManagedRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			record.State = ManagedWorkspaceMissing
			return ErrManagedWorkspaceMissing
		}
		record.State = ManagedWorkspaceInconsistent
		return err
	}
	if err := m.validateManagedAssociation(record); err != nil {
		if errors.Is(err, ErrManagedWorkspaceMissing) {
			record.State = ManagedWorkspaceMissing
		} else if errors.Is(err, ErrManagedGit) {
			record.State = ManagedWorkspaceStale
		} else {
			record.State = ManagedWorkspaceInconsistent
		}
		return err
	}
	status, err := m.runner.capture(record.ManagedRoot, "status", "--porcelain=v1", "--untracked-files=all", "--")
	if err != nil {
		record.State = ManagedWorkspaceStale
		return err
	}
	if strings.TrimSpace(status) == "" {
		record.State = ManagedWorkspaceAvailable
	} else {
		record.State = ManagedWorkspaceDirty
	}
	return nil
}

func (m *ManagedWorktreeManager) descriptorLocked(record *managedWorkspaceMetadata) ManagedWorkspaceDescriptor {
	state := record.State
	if m.activeID == record.WorkspaceID {
		state = ManagedWorkspaceActive
	}
	return ManagedWorkspaceDescriptor{WorkspaceID: record.WorkspaceID, Mode: record.Mode, State: state, Active: m.activeID == record.WorkspaceID, BaseRef: record.BaseRef, BaseSHA: record.BaseSHA, DirtySource: record.DirtySource}
}

func (m *ManagedWorktreeManager) metadataPath(workspaceID string) string {
	return filepath.Join(m.worktreesDir, workspaceID+".json")
}

func (m *ManagedWorktreeManager) writeMetadata(record *managedWorkspaceMetadata) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(m.worktreesDir, ".metadata-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	path := m.metadataPath(record.WorkspaceID)
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return ErrManagedMetadata
	}
	return os.Rename(tmpName, path)
}

func readLimitedPrivate(path string, limit int64) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", ErrManagedMetadata
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > limit {
		return "", ErrManagedMetadata
	}
	return string(data), nil
}

func managedStateError(state ManagedWorkspaceState) error {
	switch state {
	case ManagedWorkspaceMissing:
		return ErrManagedWorkspaceMissing
	case ManagedWorkspaceStale:
		return ErrManagedWorkspaceStale
	case ManagedWorkspaceInconsistent, managedWorkspaceCreating:
		return ErrManagedWorkspaceInconsistent
	case ManagedWorkspaceDirty:
		return ErrManagedWorkspaceDirty
	default:
		return ErrManagedWorkspaceNotFound
	}
}

func validManagedState(state ManagedWorkspaceState) bool {
	switch state {
	case ManagedWorkspaceAvailable, ManagedWorkspaceActive, ManagedWorkspaceDirty, ManagedWorkspaceMissing, ManagedWorkspaceStale, ManagedWorkspaceInconsistent, managedWorkspaceCreating:
		return true
	default:
		return false
	}
}

func newManagedWorkspaceID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func validWorkspaceID(id string) bool { return len(id) == 32 && validHex(id) }

func validObjectID(id string) bool {
	return (len(id) == 40 || len(id) == 64) && validHex(id)
}

func validHex(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func validManagedRef(ref string) bool {
	if ref == "" || strings.TrimSpace(ref) != ref || strings.HasPrefix(ref, "-") || filepath.IsAbs(ref) || strings.Contains(ref, "..") || strings.ContainsRune(ref, '\x00') {
		return false
	}
	for _, r := range ref {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func filepathHasPrefix(path, prefix string) bool {
	path = filepath.Clean(path)
	prefix = filepath.Clean(prefix)
	return path == prefix || strings.HasPrefix(path, prefix+string(os.PathSeparator))
}
