package programming

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func gitFixture(t *testing.T) (*workspace.Session, string) {
	t.Helper()
	root := t.TempDir()
	runGitFixture(t, root, "init", "-q")
	runGitFixture(t, root, "config", "user.email", "fixture@example.invalid")
	runGitFixture(t, root, "config", "user.name", "SignalSpace fixture")
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.23\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixture_test.go"), []byte("package fixture\n\nimport \"testing\"\n\nfunc TestFixture(t *testing.T) {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitFixture(t, root, "add", "go.mod", "file.txt", "fixture_test.go")
	runGitFixture(t, root, "commit", "-qm", "baseline")
	session, err := workspace.OpenApprovedRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, root
}

func runGitFixture(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestGitSnapshotDistinguishesProducedEditWithoutMutatingRepository(t *testing.T) {
	session, _ := gitFixture(t)
	before, err := CaptureGitSnapshot(context.Background(), session, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != "" || before.Diff != "" {
		t.Fatalf("baseline was not clean: %+v", before)
	}
	if err := session.ReplaceText("file.txt", "before\n", "after\n"); err != nil {
		t.Fatal(err)
	}
	after, err := CaptureGitSnapshot(context.Background(), session, 8192)
	if err != nil {
		t.Fatal(err)
	}
	review := CompareGitSnapshots(before, after)
	if !review.StatusChange || !review.DiffChange {
		t.Fatalf("edit was not observed: %+v", review)
	}
	if !strings.Contains(after.Status, " M file.txt") || !strings.Contains(after.Diff, "-before") || !strings.Contains(after.Diff, "+after") {
		t.Fatalf("unexpected git observation: status=%q diff=%q", after.Status, after.Diff)
	}
	if strings.Contains(after.Status, "A  ") || strings.Contains(after.Status, "M  ") {
		t.Fatalf("snapshot performed a staging mutation: %q", after.Status)
	}
	if strings.Contains(after.Diff, "baseline") {
		t.Fatal("diff review included commit mutation")
	}
}

func TestGitSnapshotRequiresLiveSessionAndValidLimit(t *testing.T) {
	session, _ := gitFixture(t)
	if _, err := CaptureGitSnapshot(context.Background(), session, maxOutputLimit+1); err != ErrInvalidDiffReview {
		t.Fatalf("invalid output limit accepted: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureGitSnapshot(context.Background(), session, 4096); err != workspace.ErrClosed {
		t.Fatalf("closed session accepted: %v", err)
	}
}

func TestGitSnapshotNeutralizesExecutableConfigAndEnvironment(t *testing.T) {
	session, root := gitFixture(t)
	marker := filepath.Join(t.TempDir(), "executed")
	hook := filepath.Join(t.TempDir(), "git-hook.sh")
	script := "#!/bin/sh\nprintf executed > '" + marker + "'\n"
	if err := os.WriteFile(hook, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	runGitFixture(t, root, "config", "core.fsmonitor", hook)

	// A configuração e o ambiente tentam instalar tanto um fsmonitor quanto
	// um diff externo. A observação não deve executar nenhum deles.
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "core.fsmonitor")
	t.Setenv("GIT_CONFIG_VALUE_0", hook)
	t.Setenv("GIT_CONFIG_KEY_1", "diff.external")
	t.Setenv("GIT_CONFIG_VALUE_1", hook)
	t.Setenv("GIT_EXTERNAL_DIFF", hook)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "foreign-index"))

	if _, err := CaptureGitSnapshot(context.Background(), session, 8192); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("git observation executed configured helper: %v", err)
	}
}
