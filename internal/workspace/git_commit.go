package workspace

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxGitCommitMessageBytes = 16 << 10

var (
	ErrGitCommitInput       = errors.New("invalid Git commit input")
	ErrGitCommitConflict    = errors.New("Git commit precondition conflict")
	ErrGitCommitNoStaged    = errors.New("Git index has no staged changes")
	ErrGitCommitUnsupported = errors.New("Git commit state is unsupported")
)

type GitCommitRequest struct {
	ExpectedHeadOID     string
	ExpectedIndexSHA256 string
	Message             string
}

type GitCommitResult struct {
	Status            string   `json:"status"`
	CommitOID         string   `json:"commit_oid,omitempty"`
	ParentOID         string   `json:"parent_oid,omitempty"`
	TreeOID           string   `json:"tree_oid,omitempty"`
	PreviousHeadOID   string   `json:"previous_head_oid,omitempty"`
	CurrentHeadOID    string   `json:"current_head_oid,omitempty"`
	StagedPathsCount  int      `json:"staged_paths_count"`
	RemainingUnstaged []string `json:"remaining_unstaged"`
	Detached          bool     `json:"detached"`
	ReachabilityState string   `json:"reachability_state"`
}

func validateGitCommitRequest(request GitCommitRequest) error {
	if !validObjectID(request.ExpectedHeadOID) || len(request.ExpectedIndexSHA256) != 64 || !validHex(request.ExpectedIndexSHA256) {
		return ErrGitCommitInput
	}
	if len(request.Message) == 0 || len(request.Message) > MaxGitCommitMessageBytes || !utf8.ValidString(request.Message) || strings.TrimSpace(request.Message) == "" {
		return ErrGitCommitInput
	}
	for _, r := range request.Message {
		if r == '\r' || r == '\x7f' || (r < 0x20 && r != '\n' && r != '\t') || (r >= 0x80 && r <= 0x9f) || unicode.IsControl(r) && r != '\n' && r != '\t' {
			return ErrGitCommitInput
		}
	}
	return nil
}

// CommitGitIndex executa o commit plumbing apenas no workspace identificado
// pelo manager. O manager conserva BaseSHA como autoridade e mantém HEAD e a
// ref privada em uma única transação update-ref.
func (m *ManagedWorktreeManager) CommitGitIndex(workspaceID string, directory ProcessDirectory, request GitCommitRequest) (GitCommitResult, error) {
	if m == nil || directory == nil {
		return GitCommitResult{Status: "failed_no_ref_change"}, ErrManagedWorkspaceNotFound
	}
	if err := validateGitCommitRequest(request); err != nil {
		return GitCommitResult{Status: "failed_no_ref_change"}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return GitCommitResult{Status: "failed_no_ref_change"}, ErrManagedWorkspaceClosed
	}
	record, ok := m.records[workspaceID]
	if !ok || record == nil {
		return GitCommitResult{Status: "failed_no_ref_change"}, ErrManagedWorkspaceNotFound
	}
	identity, err := m.gitIdentityLocked()
	if err != nil {
		return GitCommitResult{Status: "failed_no_ref_change"}, err
	}
	var result GitCommitResult
	err = directory.WithProcessDir(func(dir string) error {
		resolvedDir, resolveErr := filepath.EvalSymlinks(dir)
		if resolveErr != nil || filepath.Clean(resolvedDir) != filepath.Clean(record.ManagedRoot) {
			return ErrManagedWorktreeRequired
		}
		result, err = m.commitGitIndexLocked(dir, record, identity, request)
		return err
	})
	return result, err
}

func (m *ManagedWorktreeManager) commitGitIndexLocked(directory string, record *managedWorkspaceMetadata, identity GitIdentity, request GitCommitRequest) (GitCommitResult, error) {
	result := GitCommitResult{Status: "failed_no_ref_change", ParentOID: request.ExpectedHeadOID, PreviousHeadOID: request.ExpectedHeadOID, Detached: true, ReachabilityState: "not_committed"}
	state, err := m.inspectManagedGitState(record)
	if err != nil {
		return result, err
	}
	if state.HeadSHA != request.ExpectedHeadOID {
		result.Status = "conflict_no_ref_change"
		return result, ErrGitCommitConflict
	}
	status, err := readGitIndexStatus(directory)
	if err != nil {
		return result, ErrManagedGit
	}
	if status.IndexSHA256 != request.ExpectedIndexSHA256 {
		result.Status = "conflict_no_ref_change"
		return result, ErrGitCommitConflict
	}
	for _, entry := range status.Entries {
		if entry.Conflict {
			return result, ErrGitCommitUnsupported
		}
		if entry.Staged {
			result.StagedPathsCount++
		}
	}
	if result.StagedPathsCount == 0 {
		return result, ErrGitCommitNoStaged
	}
	if err := m.validateNoUnsupportedFeatures(directory); err != nil {
		return result, err
	}

	state, err = m.inspectManagedGitState(record)
	if err != nil || state.HeadSHA != request.ExpectedHeadOID {
		result.Status = "conflict_no_ref_change"
		return result, ErrGitCommitConflict
	}
	status, err = readGitIndexStatus(directory)
	if err != nil || status.IndexSHA256 != request.ExpectedIndexSHA256 {
		result.Status = "conflict_no_ref_change"
		return result, ErrGitCommitConflict
	}
	if output, err := m.runner.capture(directory, "diff", "--cached", "--quiet", "--exit-code", "--"); err == nil {
		return result, ErrGitCommitNoStaged
	} else if code, ok := managedGitExitCode(err); !ok || code != 1 {
		return result, ErrManagedGit
	} else if strings.TrimSpace(output) != "" {
		// stderr is intentionally not exposed; the exit code is the authority.
	}

	treeBytes, truncated, err := m.runner.captureOutput(directory, 128, "write-tree")
	if err != nil || truncated {
		return result, ErrManagedGit
	}
	treeOID := strings.TrimSpace(string(treeBytes))
	if !validObjectID(treeOID) {
		return result, ErrManagedGit
	}
	result.TreeOID = treeOID

	state, err = m.inspectManagedGitState(record)
	if err != nil || state.HeadSHA != request.ExpectedHeadOID {
		result.Status = "conflict_no_ref_change"
		return result, ErrGitCommitConflict
	}
	status, err = readGitIndexStatus(directory)
	if err != nil || status.IndexSHA256 != request.ExpectedIndexSHA256 {
		result.Status = "conflict_no_ref_change"
		return result, ErrGitCommitConflict
	}

	now := strconv.FormatInt(time.Now().UTC().Unix(), 10) + " +0000"
	commitBytes, truncated, err := m.runner.captureInputEnv(directory, []byte(request.Message), map[string]string{
		"GIT_AUTHOR_NAME":     identity.DisplayName,
		"GIT_AUTHOR_EMAIL":    identity.Email,
		"GIT_AUTHOR_DATE":     now,
		"GIT_COMMITTER_NAME":  identity.DisplayName,
		"GIT_COMMITTER_EMAIL": identity.Email,
		"GIT_COMMITTER_DATE":  now,
	}, "commit-tree", treeOID, "-p", request.ExpectedHeadOID)
	if err != nil || truncated {
		return result, ErrManagedGit
	}
	commitOID := strings.TrimSpace(string(commitBytes))
	if !validObjectID(commitOID) {
		return result, ErrManagedGit
	}
	result.CommitOID = commitOID

	ref := managedHeadRef(record.WorkspaceID)
	var transaction string
	if state.HasLocalCommits {
		transaction = "start\nupdate HEAD " + commitOID + " " + request.ExpectedHeadOID + "\nupdate " + ref + " " + commitOID + " " + request.ExpectedHeadOID + "\nprepare\ncommit\n"
	} else {
		transaction = "start\nupdate HEAD " + commitOID + " " + request.ExpectedHeadOID + "\ncreate " + ref + " " + commitOID + "\nprepare\ncommit\n"
	}
	_, _, txErr := m.runner.captureInput(directory, []byte(transaction), "update-ref", "--stdin")
	if txErr != nil {
		return m.reconcileCommitFailure(directory, record, request.ExpectedHeadOID, commitOID, state, result, txErr)
	}
	finalState, finalErr := m.readManagedGitRefs(directory, record.WorkspaceID)
	if finalErr != nil || finalState.HeadSHA != commitOID || finalState.PrivateRefSHA != commitOID || !finalState.PrivateRefSeen {
		result.Status = "partial_or_unknown"
		result.ReachabilityState = "unknown_or_inconsistent"
		return result, ErrManagedWorkspaceInconsistent
	}
	finalStatus, finalStatusErr := readGitIndexStatus(directory)
	if finalStatusErr != nil {
		result.Status = "partial_or_unknown"
		result.ReachabilityState = "private_ref_and_head_confirmed"
		return result, finalStatusErr
	}
	result.Status = "committed"
	result.CurrentHeadOID = commitOID
	result.ReachabilityState = "private_ref_and_head_confirmed"
	result.RemainingUnstaged = remainingUnstaged(finalStatus)
	return result, nil
}

func remainingUnstaged(status GitIndexStatus) []string {
	result := make([]string, 0)
	for _, entry := range status.Entries {
		if entry.Unstaged || entry.Untracked {
			result = append(result, entry.Path)
		}
	}
	return result
}

func (m *ManagedWorktreeManager) reconcileCommitFailure(directory string, record *managedWorkspaceMetadata, oldOID, newOID string, before managedGitState, result GitCommitResult, txErr error) (GitCommitResult, error) {
	after, readErr := m.readManagedGitRefs(directory, record.WorkspaceID)
	if readErr == nil && after.HeadSHA == newOID && after.PrivateRefSHA == newOID && after.PrivateRefSeen {
		result.Status = "committed"
		result.CurrentHeadOID = newOID
		result.ReachabilityState = "private_ref_and_head_confirmed"
		return result, nil
	}
	if readErr == nil && after.HeadSHA == oldOID && ((before.PrivateRefSeen && after.PrivateRefSHA == oldOID) || (!before.PrivateRefSeen && !after.PrivateRefSeen)) {
		result.Status = "conflict_no_ref_change"
		return result, errors.Join(ErrGitCommitConflict, txErr)
	}
	result.Status = "partial_or_unknown"
	result.ReachabilityState = "unknown_or_inconsistent"
	return result, errors.Join(ErrManagedWorkspaceInconsistent, txErr)
}

func (m *ManagedWorktreeManager) readManagedGitRefs(directory, workspaceID string) (managedGitState, error) {
	headBytes, headTruncated, headErr := m.runner.captureOutput(directory, 128, "rev-parse", "--verify", "--end-of-options", "HEAD^{commit}")
	if headErr != nil || headTruncated || !validObjectID(strings.TrimSpace(string(headBytes))) {
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	}
	refBytes, refTruncated, refErr := m.runner.captureOutput(directory, 128, "rev-parse", "--verify", "--end-of-options", managedHeadRef(workspaceID)+"^{commit}")
	if refTruncated {
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	}
	state := managedGitState{HeadSHA: strings.TrimSpace(string(headBytes))}
	if refErr == nil {
		state.PrivateRefSHA = strings.TrimSpace(string(refBytes))
		state.PrivateRefSeen = validObjectID(state.PrivateRefSHA)
		if !state.PrivateRefSeen {
			return managedGitState{}, ErrManagedWorkspaceInconsistent
		}
		return state, nil
	}
	if code, ok := managedGitExitCode(refErr); !ok || code != 128 || len(refBytes) != 0 {
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	}
	return state, nil
}
