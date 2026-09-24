package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManagedGitIndexStageUnstageUsesTypedPreconditions(t *testing.T) {
	source := managedGitTestRepo(t)
	state := filepath.Join(t.TempDir(), "state")
	manager, err := NewManagedWorktreeManager(state)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	descriptor, err := manager.Create(source, "")
	if err != nil {
		t.Fatal(err)
	}
	grants, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	sessionID, _, err := manager.Activate(descriptor.WorkspaceID, managedTestClient, grants, ScopeRead, ScopeGit, ScopeGitIndex)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = grants.Revoke(sessionID)
		_ = manager.Deactivate(descriptor.WorkspaceID)
	}()
	managedRoot := filepath.Join(state, "worktrees", descriptor.WorkspaceID)
	if err := os.WriteFile(filepath.Join(managedRoot, "tracked.txt"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var status GitIndexStatus
	if err := grants.WithAuthorizedGitProcessDir("local-owner", managedTestClient, sessionID, func(directory ProcessDirectory) error {
		var err error
		status, err = CaptureGitIndexStatus(directory)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	entry := findGitStatusEntry(t, status, "tracked.txt")
	if !entry.Unstaged || !entry.Modified || entry.Staged {
		t.Fatalf("unexpected unstaged state: %+v", entry)
	}
	expectedSHA := fileSHA256(t, filepath.Join(managedRoot, "tracked.txt"))
	var staged GitIndexMutationResult
	if err := grants.WithAuthorizedManagedGitProcessDir("local-owner", managedTestClient, sessionID, func(directory ProcessDirectory) error {
		var err error
		staged, err = StageGitPaths(directory, status.IndexSHA256, []GitIndexEntry{{Path: "tracked.txt", ExpectedSHA256: expectedSHA}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if staged.Status != "staged" || staged.NewIndexSHA256 == staged.OldIndexSHA256 || staged.Partial {
		t.Fatalf("stage did not report a clean mutation: %+v", staged)
	}
	var stagedStatus GitIndexStatus
	if err := grants.WithAuthorizedGitProcessDir("local-owner", managedTestClient, sessionID, func(directory ProcessDirectory) error {
		var err error
		stagedStatus, err = CaptureGitIndexStatus(directory)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	stagedEntry := findGitStatusEntry(t, stagedStatus, "tracked.txt")
	if !stagedEntry.Staged || stagedEntry.IndexObjectID == "" {
		t.Fatalf("stage state missing: %+v", stagedEntry)
	}
	if err := os.WriteFile(filepath.Join(managedRoot, "tracked.txt"), []byte("changed again\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := grants.WithAuthorizedManagedGitProcessDir("local-owner", managedTestClient, sessionID, func(directory ProcessDirectory) error {
		_, err := StageGitPaths(directory, stagedStatus.IndexSHA256, []GitIndexEntry{{Path: "tracked.txt", ExpectedSHA256: expectedSHA}})
		return err
	}); !errors.Is(err, ErrGitWorktreeStale) {
		t.Fatalf("stale working hash was accepted: %v", err)
	}
	workingBytes, err := os.ReadFile(filepath.Join(managedRoot, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := grants.WithAuthorizedManagedGitProcessDir("local-owner", managedTestClient, sessionID, func(directory ProcessDirectory) error {
		var result GitIndexMutationResult
		result, err = UnstageGitPaths(directory, stagedStatus.IndexSHA256, []GitIndexEntry{{Path: "tracked.txt", ExpectedIndexOID: stagedEntry.IndexObjectID}})
		if err == nil && (result.Status != "unstaged" || result.Partial) {
			err = errors.New("unexpected unstage result")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if after, err := os.ReadFile(filepath.Join(managedRoot, "tracked.txt")); err != nil || string(after) != string(workingBytes) {
		t.Fatalf("unstage changed working tree: %q %v", after, err)
	}
}

func TestGitIndexMutationRequiresManagedWorktree(t *testing.T) {
	root := managedGitTestRepo(t)
	grants, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	sessionID, err := grants.GrantWithScopes(root, managedTestClient, ScopeRead, ScopeGitIndex)
	if err != nil {
		t.Fatal(err)
	}
	if err := grants.WithAuthorizedManagedGitProcessDir("local-owner", managedTestClient, sessionID, func(ProcessDirectory) error { return nil }); !errors.Is(err, ErrManagedWorktreeRequired) {
		t.Fatalf("checkout session reached index mutation: %v", err)
	}
}

func findGitStatusEntry(t *testing.T, status GitIndexStatus, path string) GitIndexStatusEntry {
	t.Helper()
	for _, entry := range status.Entries {
		if entry.Path == path {
			return entry
		}
	}
	t.Fatalf("Git status entry %q not found: %+v", path, status)
	return GitIndexStatusEntry{}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
