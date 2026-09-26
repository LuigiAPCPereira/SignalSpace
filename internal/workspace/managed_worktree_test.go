package workspace

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
)

const managedTestClient = "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"

func TestManagedWorktreeCreateActivateRevokeResumeAndRemove(t *testing.T) {
	source := managedGitTestRepo(t)
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("dirty source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "untracked.txt"), []byte("must not copy\n"), 0600); err != nil {
		t.Fatal(err)
	}
	hookMarker := filepath.Join(t.TempDir(), "hook-ran")
	hook := filepath.Join(source, ".git", "hooks", "post-checkout")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf ran > "+shellQuote(hookMarker)+"\n"), 0700); err != nil {
		t.Fatal(err)
	}

	state := filepath.Join(t.TempDir(), "state")
	manager, err := NewManagedWorktreeManager(state)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.ValidateCreateRequest(source, ""); err != nil {
		t.Fatalf("request validation: %v", err)
	}
	descriptor, err := manager.Create(source, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if descriptor.Mode != WorkspaceModeWorktree || descriptor.State != ManagedWorkspaceAvailable || !descriptor.DirtySource || len(descriptor.WorkspaceID) != 32 || strings.Contains(descriptor.WorkspaceID, "/") {
		t.Fatalf("unsafe or incomplete descriptor: %#v", descriptor)
	}
	managedRoot := filepath.Join(state, "worktrees", descriptor.WorkspaceID)
	if _, err := os.Stat(filepath.Join(managedRoot, "untracked.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("untracked source file copied: %v", err)
	}
	if _, err := os.Stat(hookMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkout hook ran: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(managedRoot, "tracked.txt")); err != nil || string(content) != "baseline\n" {
		t.Fatalf("materialized base: %q %v", content, err)
	}

	grants, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	sessionID, active, err := manager.Activate(descriptor.WorkspaceID, managedTestClient, grants, ScopeRead, ScopeWrite, ScopeGit)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if active.State != ManagedWorkspaceActive || !active.Active {
		t.Fatalf("active descriptor: %#v", active)
	}
	if content, err := grants.ReadText("local-owner", managedTestClient, sessionID, "tracked.txt"); err != nil || content != "baseline\n" {
		t.Fatalf("managed session read: %q %v", content, err)
	}
	if err := grants.Revoke(sessionID); err != nil {
		t.Fatal(err)
	}
	if err := manager.Deactivate(descriptor.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	resumedSession, resumed, err := manager.Activate(descriptor.WorkspaceID, managedTestClient, grants, ScopeRead)
	if err != nil {
		t.Fatalf("resume activation: %v", err)
	}
	if resumedSession == sessionID || resumed.State != ManagedWorkspaceActive {
		t.Fatalf("resume reused session or stayed inactive: old=%s new=%s %#v", sessionID, resumedSession, resumed)
	}
	if err := grants.Revoke(resumedSession); err != nil {
		t.Fatal(err)
	}
	if err := manager.Deactivate(descriptor.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(descriptor.WorkspaceID); err != nil {
		t.Fatalf("clean remove: %v", err)
	}
	if _, err := manager.Descriptor(descriptor.WorkspaceID); !errors.Is(err, ErrManagedWorkspaceNotFound) {
		t.Fatalf("removed metadata still available: %v", err)
	}
	if _, err := os.Stat(managedRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed root remains: %v", err)
	}
}

func TestActivateProgrammingUsesManagedCapabilityEnvelope(t *testing.T) {
	source := managedGitTestRepo(t)
	state := filepath.Join(t.TempDir(), "state")
	manager, err := NewManagedWorktreeManager(state)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
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
	sessionID, _, err := manager.ActivateProgramming(descriptor.WorkspaceID, managedTestClient, grants)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := grants.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	want := ProgrammingManagedCapabilities()
	if len(snapshot.Capabilities) != len(want) {
		t.Fatalf("managed Programming capabilities = %v, want %v", snapshot.Capabilities, want)
	}
	for index, value := range want {
		if snapshot.Capabilities[index] != value {
			t.Fatalf("managed Programming capability[%d] = %q, want %q", index, snapshot.Capabilities[index], value)
		}
	}
	if grants.AllowsClientCapability(managedTestClient, capability.TestRun) {
		t.Fatal("managed Programming envelope leaked test.run")
	}
	if err := grants.Revoke(sessionID); err != nil {
		t.Fatal(err)
	}
}

func TestManagedWorktreeRestartDirtyAndActiveRemovalFailClosed(t *testing.T) {
	source := managedGitTestRepo(t)
	state := filepath.Join(t.TempDir(), "state")
	manager, err := NewManagedWorktreeManager(state)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := manager.Create(source, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	managedRoot := filepath.Join(state, "worktrees", descriptor.WorkspaceID)
	if err := os.WriteFile(filepath.Join(managedRoot, "local-change"), []byte("dirty\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(descriptor.WorkspaceID); !errors.Is(err, ErrManagedWorkspaceDirty) {
		t.Fatalf("dirty remove was accepted: %v", err)
	}
	grants, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	sessionID, _, err := manager.Activate(descriptor.WorkspaceID, managedTestClient, grants, ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(descriptor.WorkspaceID); !errors.Is(err, ErrManagedWorkspaceActive) {
		t.Fatalf("active remove was accepted: %v", err)
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
	restarted, err := NewManagedWorktreeManager(state)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	items, err := restarted.List()
	if err != nil || len(items) != 1 || items[0].WorkspaceID != descriptor.WorkspaceID || items[0].State != ManagedWorkspaceDirty {
		t.Fatalf("restart did not preserve dirty metadata: %#v %v", items, err)
	}
}

func TestManagedWorktreeRejectsInvalidSourcesAndFilters(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	manager, err := NewManagedWorktreeManager(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ValidateCreateRequest(t.TempDir(), ""); !errors.Is(err, ErrInvalidManagedSource) {
		t.Fatalf("non-Git source accepted: %v", err)
	}
	if err := manager.ValidateCreateRequest("/", ""); !errors.Is(err, ErrInvalidManagedSource) {
		t.Fatalf("root source accepted: %v", err)
	}
	filtered := managedGitTestRepo(t)
	if err := runManagedTestGit(filtered, "config", "filter.secret.clean", "cat"); err != nil {
		t.Fatal(err)
	}
	if err := manager.ValidateCreateRequest(filtered, ""); !errors.Is(err, ErrUnsupportedWorktreeFilter) {
		t.Fatalf("filter source accepted: %v", err)
	}
	if _, err := manager.Create(filtered, "does-not-exist"); !errors.Is(err, ErrUnsupportedWorktreeFilter) {
		t.Fatalf("invalid base bypassed filter/source gate: %v", err)
	}
}

func TestManagedSessionReservesGitMetadata(t *testing.T) {
	source := managedGitTestRepo(t)
	state := filepath.Join(t.TempDir(), "state")
	manager, err := NewManagedWorktreeManager(state)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := manager.Create(source, "")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(state, "worktrees", descriptor.WorkspaceID)
	session, err := OpenManagedRoot(root, WorkspaceMetadata{Mode: WorkspaceModeWorktree, ManagedWorkspaceID: descriptor.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	checks := []func() error{
		func() error { _, err := session.ReadText(".git/config"); return err },
		func() error { _, err := session.StatPath(".git"); return err },
		func() error { _, err := session.ListDirectory(".git"); return err },
		func() error { _, err := session.FindPaths(".git", "*", 10, 2); return err },
		func() error { _, err := session.SearchText(".git", "core", 10); return err },
		func() error { _, err := session.Copy(".git/config", "copy"); return err },
		func() error { _, err := session.Move(".git/config", "moved"); return err },
		func() error { _, err := session.DeleteFile(".git/config"); return err },
		func() error { _, err := session.CreateDirectory(".git/new"); return err },
		func() error { _, err := session.CreateTextFile(".git/new", "x"); return err },
		func() error { _, err := session.WriteTextFile(".git/config", "bad", "x"); return err },
		func() error { return session.ReplaceText(".git/config", "", "x") },
		func() error {
			_, err := session.ApplyPatch([]PatchOperation{{Type: PatchDeleteFile, Path: ".git/config", ExpectedSHA256: strings.Repeat("0", 64)}})
			return err
		},
	}
	for index, check := range checks {
		if err := check(); !errors.Is(err, ErrReservedPath) {
			t.Errorf("reserved operation %d returned %v", index, err)
		}
	}
	if names, err := session.ListDirectory("."); err != nil || containsString(names, ".git") {
		t.Fatalf("root listing leaked .git: %v %v", names, err)
	}
}

func managedGitTestRepo(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := runManagedTestGit(root, "init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"config", "user.email", "signalspace-test@example.invalid"}, {"config", "user.name", "SignalSpace test"}} {
		if err := runManagedTestGit(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("baseline\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runManagedTestGit(root, "add", "tracked.txt"); err != nil {
		t.Fatal(err)
	}
	if err := runManagedTestGit(root, "commit", "--quiet", "-m", "baseline"); err != nil {
		t.Fatal(err)
	}
	return root
}

func runManagedTestGit(directory string, args ...string) error {
	command := exec.Command("git", args...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0")
	if output, err := command.CombinedOutput(); err != nil {
		return errors.Join(err, errors.New(strings.TrimSpace(string(output))))
	}
	return nil
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
