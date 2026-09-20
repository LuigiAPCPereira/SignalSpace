package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func testWorkspaceConsole(t *testing.T) *workspaceConsole {
	t.Helper()
	grants, err := workspace.NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	console := &workspaceConsole{grants: grants}
	t.Cleanup(func() { _ = console.Close() })
	return console
}

func TestWorkspaceConsoleRequiresSeparateLocalConfirmation(t *testing.T) {
	console := testWorkspaceConsole(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("approved"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if console.handleWorkspaceCommand("approve oauth-request", &out) {
		t.Fatal("non-workspace command captured")
	}
	if !console.handleWorkspaceCommand("workspace request "+root, &out) || console.pending == nil {
		t.Fatal("no pending request")
	}
	pendingID := console.pending.id
	if _, err := console.grants.ReadText("local-owner", pendingID, "file"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("unconfirmed root readable: %v", err)
	}
	if !strings.Contains(out.String(), "workspace approve "+pendingID) || !strings.Contains(out.String(), root) {
		t.Fatalf("missing exact consent request: %s", out.String())
	}
	console.handleWorkspaceCommand("workspace approve wrong", &out)
	if console.pending != nil {
		t.Fatal("invalid approval preserved request")
	}
	console.handleWorkspaceCommand("workspace request "+root, &out)
	pendingID = console.pending.id
	console.handleWorkspaceCommand("workspace approve "+pendingID, &out)
	if console.pending != nil {
		t.Fatal("approved request not cleared")
	}
	if !strings.Contains(out.String(), "Local workspace grant created:") {
		t.Fatalf("approval not reported: %s", out.String())
	}
	// A concessão existe apenas no gerenciador local; nunca no endpoint HTTP.
	id := strings.Split(strings.Split(out.String(), "session=")[1], ".")[0]
	if _, err := console.grants.ReadText("other-owner", id, "file"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("other identity authorized: %v", err)
	}
	if text, err := console.grants.ReadText("local-owner", id, "file"); err != nil || text != "approved" {
		t.Fatalf("approved read: %q, %v", text, err)
	}
	console.handleWorkspaceCommand("workspace revoke "+id, &out)
	if _, err := console.grants.ReadText("local-owner", id, "file"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("revoke failed: %v", err)
	}
}

func TestWorkspaceConsoleCancelsExpiresAndRejectsBroadRoots(t *testing.T) {
	console := testWorkspaceConsole(t)
	var out bytes.Buffer
	for _, root := range []string{"/", ".", "/tmp/../etc", "relative/path", strings.Repeat("a", 4097)} {
		console.handleWorkspaceCommand("workspace request "+root, &out)
		if console.pending != nil {
			t.Fatalf("accepted invalid root %q", root)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		console.handleWorkspaceCommand("workspace request "+home, &out)
		if console.pending != nil {
			t.Fatal("accepted home")
		}
	}
	root := t.TempDir()
	console.handleWorkspaceCommand("workspace request "+root, &out)
	id := console.pending.id
	console.handleWorkspaceCommand("workspace cancel "+id, &out)
	if console.pending != nil {
		t.Fatal("cancel retained pending grant")
	}
	console.handleWorkspaceCommand("workspace approve "+id, &out)
	console.handleWorkspaceCommand("workspace request "+root, &out)
	id = console.pending.id
	console.pending.expires = time.Now().Add(-time.Second)
	console.handleWorkspaceCommand("workspace approve "+id, &out)
	if console.pending != nil {
		t.Fatal("expired consent retained")
	}
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	console.handleWorkspaceCommand("workspace request "+link, &out)
	id = console.pending.id
	console.handleWorkspaceCommand("workspace approve "+id, &out)
	if !strings.Contains(out.String(), "workspace grant rejected") {
		t.Fatal("symlink accepted")
	}
	if _, err := console.grants.ReadText("local-owner", id, "file"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("symlink root granted: %v", err)
	}
}

func TestWorkspaceConsoleShutdownRevokes(t *testing.T) {
	console := testWorkspaceConsole(t)
	root := t.TempDir()
	var out bytes.Buffer
	console.handleWorkspaceCommand("workspace request "+root, &out)
	console.handleWorkspaceCommand("workspace approve "+console.pending.id, &out)
	if err := console.Close(); err != nil {
		t.Fatal(err)
	}
	console.handleWorkspaceCommand("workspace request "+root, &out)
	console.handleWorkspaceCommand("workspace approve "+console.pending.id, &out)
	if !strings.Contains(out.String(), "workspace grant rejected: workspace session closed") {
		t.Fatalf("post-close grant accepted: %s", out.String())
	}
}
