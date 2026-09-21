package programming

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func TestLocalProgrammingVerticalSliceReadEditTestDiffRevoke(t *testing.T) {
	session, workingRoot := gitFixture(t)
	grants, err := workspace.NewGrants("owner")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grants.Close() })
	if err := os.WriteFile(filepath.Join(workingRoot, "workspace.txt"), []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitFixture(t, workingRoot, "add", "workspace.txt")
	runGitFixture(t, workingRoot, "commit", "-qm", "workspace baseline")
	grantID, err := grants.Grant(workingRoot, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}

	before, err := CaptureGitSnapshot(context.Background(), session, 8192)
	if err != nil {
		t.Fatal(err)
	}
	content, err := grants.ReadText("owner", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", grantID, "workspace.txt")
	if err != nil || content != "original\n" {
		t.Fatalf("read: %q, %v", content, err)
	}
	if err := grants.ReplaceText("owner", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", grantID, "workspace.txt", content, "edited\n"); err != nil {
		t.Fatal(err)
	}
	result, err := RunPredefinedTest(context.Background(), session, 30*time.Second, 8192)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("predefined test: %+v, %v", result, err)
	}
	after, err := CaptureGitSnapshot(context.Background(), session, 8192)
	if err != nil {
		t.Fatal(err)
	}
	review := CompareGitSnapshots(before, after)
	if !review.StatusChange || !review.DiffChange {
		t.Fatalf("vertical edit was not observable: %+v", review)
	}

	if err := grants.Revoke(grantID); err != nil {
		t.Fatal(err)
	}
	if err := grants.ReplaceText("owner", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", grantID, "workspace.txt", "edited\n", "denied\n"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("edit after grant revoke: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := session.ReplaceText("workspace.txt", "edited\n", "denied\n"); !errors.Is(err, workspace.ErrClosed) {
		t.Fatalf("edit after session close: %v", err)
	}
}
