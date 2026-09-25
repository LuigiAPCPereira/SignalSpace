package workspace

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitIdentityIsPrivateAndCommitRequiresIt(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	manager, err := NewManagedWorktreeManager(state)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if _, err := manager.GitIdentity(); !errors.Is(err, ErrGitIdentityMissing) {
		t.Fatalf("missing identity returned %v", err)
	}
	for _, invalid := range [][2]string{{"", "Name"}, {"mail\n@example.invalid", "Name"}, {"mail@example.invalid", "<Name>"}, {"mail@example.invalid", strings.Repeat("n", maxGitIdentityName+1)}} {
		if err := manager.SetGitIdentity(invalid[0], invalid[1]); !errors.Is(err, ErrGitIdentityInvalid) {
			t.Fatalf("invalid identity %q/%q returned %v", invalid[0], invalid[1], err)
		}
	}
	if err := manager.SetGitIdentity("owner@example.invalid", "Owner Local"); err != nil {
		t.Fatal(err)
	}
	identity, err := manager.GitIdentity()
	if err != nil || identity.Email != "owner@example.invalid" || identity.DisplayName != "Owner Local" {
		t.Fatalf("stored identity = %#v, err=%v", identity, err)
	}
	path := filepath.Join(state, gitIdentityFilename)
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("identity permissions = %v %v", info, err)
	}
	if err := manager.ClearGitIdentity(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.GitIdentity(); !errors.Is(err, ErrGitIdentityMissing) {
		t.Fatalf("cleared identity returned %v", err)
	}
}

func TestManagedGitCommitCreatesPrivateRefAndSurvivesResume(t *testing.T) {
	source := managedGitTestRepo(t)
	stateDir := filepath.Join(t.TempDir(), "state")
	manager, err := NewManagedWorktreeManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetGitIdentity("owner@example.invalid", "Owner Local"); err != nil {
		t.Fatal(err)
	}
	descriptor, err := manager.Create(source, "")
	if err != nil {
		t.Fatal(err)
	}
	grants, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	client := managedTestClient
	sessionID, _, err := manager.Activate(descriptor.WorkspaceID, client, grants, ScopeRead, ScopeWrite, ScopeGit, ScopeGitIndex, ScopeGitCommit)
	if err != nil {
		t.Fatal(err)
	}
	foreignDirectory, err := OpenApprovedRoot(source)
	if err != nil {
		t.Fatal(err)
	}
	defer foreignDirectory.Close()
	if _, err := manager.CommitGitIndex(descriptor.WorkspaceID, foreignDirectory, GitCommitRequest{ExpectedHeadOID: descriptor.BaseSHA, ExpectedIndexSHA256: strings.Repeat("0", 64), Message: "must stay managed"}); !errors.Is(err, ErrManagedWorktreeRequired) {
		t.Fatalf("commit accepted a non-managed directory: %v", err)
	}
	managedRoot := filepath.Join(stateDir, "worktrees", descriptor.WorkspaceID)
	if err := os.WriteFile(filepath.Join(managedRoot, "tracked.txt"), []byte("committed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var before GitIndexStatus
	if err := grants.WithAuthorizedGitProcessDir("local-owner", client, sessionID, func(directory ProcessDirectory) error {
		var err error
		before, err = CaptureGitIndexStatus(directory)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := grants.WithAuthorizedManagedGitProcessDir("local-owner", client, sessionID, func(directory ProcessDirectory) error {
		_, err := StageGitPaths(directory, before.IndexSHA256, []GitIndexEntry{{Path: "tracked.txt", ExpectedSHA256: fileSHA256(t, filepath.Join(managedRoot, "tracked.txt"))}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managedRoot, "tracked.txt"), []byte("unstaged-after-commit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var afterStage GitIndexStatus
	if err := grants.WithAuthorizedGitProcessDir("local-owner", client, sessionID, func(directory ProcessDirectory) error {
		var err error
		afterStage, err = CaptureGitIndexStatus(directory)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	oldHead := gitTestOutput(t, managedRoot, "rev-parse", "HEAD")
	var result GitCommitResult
	if err := grants.WithAuthorizedManagedGitCommit("local-owner", client, sessionID, func(directory ProcessDirectory, metadata WorkspaceMetadata) error {
		var err error
		result, err = manager.CommitGitIndex(metadata.ManagedWorkspaceID, directory, GitCommitRequest{ExpectedHeadOID: oldHead, ExpectedIndexSHA256: afterStage.IndexSHA256, Message: "typed local commit\nsecond line"})
		return err
	}); err != nil {
		t.Fatalf("commit: %v result=%+v", err, result)
	}
	if result.Status != "committed" || result.CommitOID == "" || result.ParentOID != oldHead || result.TreeOID == "" || !result.Detached || len(result.RemainingUnstaged) != 1 || result.RemainingUnstaged[0] != "tracked.txt" {
		t.Fatalf("unexpected commit result: %+v", result)
	}
	newHead := gitTestOutput(t, managedRoot, "rev-parse", "HEAD")
	if newHead != result.CommitOID || gitTestOutput(t, managedRoot, "rev-parse", managedHeadRef(descriptor.WorkspaceID)) != newHead {
		t.Fatalf("HEAD/private ref mismatch: head=%s ref=%s result=%+v", newHead, gitTestOutput(t, managedRoot, "rev-parse", managedHeadRef(descriptor.WorkspaceID)), result)
	}
	if got := gitTestOutput(t, managedRoot, "show", "-s", "--format=%B", newHead); got != "typed local commit\nsecond line" {
		t.Fatalf("commit message changed: %q", got)
	}
	if got := gitTestOutput(t, managedRoot, "show", "-s", "--format=%an <%ae>", newHead); got != "Owner Local <owner@example.invalid>" {
		t.Fatalf("commit identity changed: %q", got)
	}
	if err := os.WriteFile(filepath.Join(managedRoot, "tracked.txt"), []byte("committed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.ClearGitIdentity(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CommitGitIndex(descriptor.WorkspaceID, grants.current, GitCommitRequest{ExpectedHeadOID: newHead, ExpectedIndexSHA256: afterStage.IndexSHA256, Message: "must fail"}); !errors.Is(err, ErrGitIdentityMissing) {
		t.Fatalf("commit without identity returned %v", err)
	}
	if err := manager.SetGitIdentity("owner@example.invalid", "Owner Local"); err != nil {
		t.Fatal(err)
	}
	if result, err := manager.CommitGitIndex(descriptor.WorkspaceID, grants.current, GitCommitRequest{ExpectedHeadOID: oldHead, ExpectedIndexSHA256: afterStage.IndexSHA256, Message: "stale head"}); !errors.Is(err, ErrGitCommitConflict) || result.Status != "conflict_no_ref_change" {
		t.Fatalf("stale head was not rejected by CAS: result=%+v err=%v", result, err)
	}
	if result, err := manager.CommitGitIndex(descriptor.WorkspaceID, grants.current, GitCommitRequest{ExpectedHeadOID: newHead, ExpectedIndexSHA256: afterStage.IndexSHA256, Message: "no staged changes"}); !errors.Is(err, ErrGitCommitNoStaged) || result.Status != "failed_no_ref_change" {
		t.Fatalf("empty index was not rejected: result=%+v err=%v", result, err)
	}
	if err := manager.ClearGitIdentity(); err != nil {
		t.Fatal(err)
	}
	if err := grants.Revoke(sessionID); err != nil {
		t.Fatal(err)
	}
	if err := manager.Deactivate(descriptor.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewManagedWorktreeManager(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	items, err := restarted.List()
	if err != nil || len(items) != 1 || !items[0].HasLocalCommits || items[0].HeadSHA != newHead {
		t.Fatalf("restart lost local commit state: %#v %v", items, err)
	}
	if err := restarted.SetGitIdentity("owner@example.invalid", "Owner Local"); err != nil {
		t.Fatal(err)
	}
	resumedSession, resumed, err := restarted.Activate(descriptor.WorkspaceID, client, grants, ScopeRead, ScopeGitCommit)
	if err != nil || resumedSession == sessionID || !resumed.HasLocalCommits {
		t.Fatalf("resume failed: session=%s descriptor=%#v err=%v", resumedSession, resumed, err)
	}
	if err := grants.WithAuthorizedManagedGitCommit("local-owner", client, sessionID, func(ProcessDirectory, WorkspaceMetadata) error { return nil }); !errors.Is(err, ErrManagedWorktreeRequired) {
		t.Fatalf("old session remained authorized: %v", err)
	}
	if err := grants.Revoke(resumedSession); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Deactivate(descriptor.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Remove(descriptor.WorkspaceID); !errors.Is(err, ErrManagedWorkspaceLocalCommits) {
		t.Fatalf("local commit worktree was removable: %v", err)
	}
}

func gitTestOutput(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_NO_REPLACE_OBJECTS=1")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(output))
}
