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
