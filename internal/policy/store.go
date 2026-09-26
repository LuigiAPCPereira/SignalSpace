package policy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
)

const (
	policyStoreVersion = 1
	maxStoredPolicies  = 128
)

var (
	ErrInvalidStore     = errors.New("invalid policy store")
	ErrStoreUnavailable = errors.New("policy store unavailable")
	ErrPolicyNotFound   = errors.New("policy not found")
)

// StoredPolicy contém somente a identidade necessária para revalidar uma
// permissão ALLOW_WORKSPACE. Não grava root, sessão, token, fingerprint ou
// conteúdo de arquivo.
type StoredPolicy struct {
	ID          string                `json:"id"`
	OwnerID     string                `json:"owner_id"`
	ClientID    string                `json:"client_id"`
	WorkspaceID string                `json:"workspace_id"`
	Capability  capability.Capability `json:"capability"`
	Effect      Effect                `json:"effect"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	ExpiresAt   *time.Time            `json:"expires_at,omitempty"`
}

type policyStoreFile struct {
	Version  int            `json:"version"`
	Policies []StoredPolicy `json:"policies"`
}

type Store struct {
	path string
}

func OpenStore(path string) (*Store, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return nil, ErrStoreUnavailable
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, errors.Join(ErrStoreUnavailable, err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, errors.Join(ErrStoreUnavailable, err)
	}
	store := &Store{path: path}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return nil, ErrStoreUnavailable
		}
		if _, err := store.List(); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.Join(ErrStoreUnavailable, err)
	}
	return store, nil
}

func (s *Store) List() ([]StoredPolicy, error) {
	if s == nil || s.path == "" {
		return nil, ErrStoreUnavailable
	}
	lock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock(lock)
	items, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	return clonePolicies(items), nil
}

func (s *Store) Upsert(policy StoredPolicy) (StoredPolicy, error) {
	if s == nil || s.path == "" {
		return StoredPolicy{}, ErrStoreUnavailable
	}
	lock, err := s.lock()
	if err != nil {
		return StoredPolicy{}, err
	}
	defer unlock(lock)
	items, err := s.loadLocked()
	if err != nil {
		return StoredPolicy{}, err
	}
	now := time.Now().UTC()
	if policy.ID == "" {
		policy.ID, err = randomPolicyID()
		if err != nil {
			return StoredPolicy{}, errors.Join(ErrStoreUnavailable, err)
		}
	}
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = now
	}
	policy.UpdatedAt = now
	if err := validateStoredPolicy(policy); err != nil {
		return StoredPolicy{}, err
	}
	for index := range items {
		if items[index].ID == policy.ID {
			policy.CreatedAt = items[index].CreatedAt
			items[index] = policy
			return policy, s.writeLocked(items)
		}
		if items[index].OwnerID == policy.OwnerID && items[index].ClientID == policy.ClientID && items[index].WorkspaceID == policy.WorkspaceID && items[index].Capability == policy.Capability {
			policy.ID = items[index].ID
			policy.CreatedAt = items[index].CreatedAt
			items[index] = policy
			return policy, s.writeLocked(items)
		}
	}
	if len(items) >= maxStoredPolicies {
		return StoredPolicy{}, ErrStoreUnavailable
	}
	items = append(items, policy)
	return policy, s.writeLocked(items)
}

func (s *Store) Delete(id string) error {
	if s == nil || s.path == "" || id == "" {
		return ErrPolicyNotFound
	}
	lock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock(lock)
	items, err := s.loadLocked()
	if err != nil {
		return err
	}
	for index := range items {
		if items[index].ID == id {
			items = append(items[:index], items[index+1:]...)
			return s.writeLocked(items)
		}
	}
	return ErrPolicyNotFound
}

func (s *Store) lock() (*os.File, error) {
	lock, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errors.Join(ErrStoreUnavailable, err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		_ = lock.Close()
		return nil, errors.Join(ErrStoreUnavailable, err)
	}
	return lock, nil
}

func unlock(lock *os.File) {
	if lock == nil {
		return
	}
	_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	_ = lock.Close()
}

func (s *Store) loadLocked() ([]StoredPolicy, error) {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Join(ErrStoreUnavailable, err)
	}
	defer file.Close()
	var document policyStoreFile
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, errors.Join(ErrInvalidStore, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.Join(ErrInvalidStore, err)
	}
	if document.Version != policyStoreVersion || len(document.Policies) > maxStoredPolicies {
		return nil, ErrInvalidStore
	}
	seen := make(map[string]struct{}, len(document.Policies))
	for _, item := range document.Policies {
		if _, ok := seen[item.ID]; ok {
			return nil, ErrInvalidStore
		}
		seen[item.ID] = struct{}{}
		if err := validateStoredPolicy(item); err != nil {
			return nil, errors.Join(ErrInvalidStore, err)
		}
	}
	return document.Policies, nil
}

func (s *Store) writeLocked(items []StoredPolicy) error {
	document := policyStoreFile{Version: policyStoreVersion, Policies: items}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".signalspace-policies-*")
	if err != nil {
		return errors.Join(ErrStoreUnavailable, err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err == nil {
		encoder := json.NewEncoder(temporary)
		encoder.SetEscapeHTML(true)
		err = encoder.Encode(document)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return errors.Join(ErrStoreUnavailable, err)
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return errors.Join(ErrStoreUnavailable, err)
	}
	return os.Chmod(s.path, 0o600)
}

func validateStoredPolicy(item StoredPolicy) error {
	if item.ID == "" || item.OwnerID == "" || item.ClientID == "" || item.WorkspaceID == "" ||
		!capability.IsKnown(item.Capability) || item.Effect != EffectAllowWorkspace || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
		return ErrInvalidStore
	}
	return nil
}

func randomPolicyID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "pol_" + hex.EncodeToString(raw[:]), nil
}

func clonePolicies(items []StoredPolicy) []StoredPolicy {
	return append([]StoredPolicy(nil), items...)
}
