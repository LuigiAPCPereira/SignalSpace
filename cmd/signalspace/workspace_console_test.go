package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

const testConsoleClient = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const otherConsoleClient = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"

func testWorkspaceConsole(t *testing.T) *workspaceConsole {
	t.Helper()
	grants, err := workspace.NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	console := &workspaceConsole{grants: grants, issuedClients: func() []auth.ClientInfo {
		return []auth.ClientInfo{{ID: testConsoleClient, Name: "Cliente de teste"}}
	}}
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
	if !console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+root, &out) || console.pending == nil {
		t.Fatal("no pending request")
	}
	pendingID := console.pending.id
	if _, err := console.grants.ReadText("local-owner", testConsoleClient, pendingID, "file"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("unconfirmed root readable: %v", err)
	}
	if !strings.Contains(out.String(), "workspace approve "+pendingID) || !strings.Contains(out.String(), root) || !strings.Contains(out.String(), testConsoleClient) {
		t.Fatalf("missing exact consent request: %s", out.String())
	}
	console.handleWorkspaceCommand("workspace approve wrong", &out)
	if console.pending != nil {
		t.Fatal("invalid approval preserved request")
	}
	console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+root, &out)
	pendingID = console.pending.id
	console.handleWorkspaceCommand("workspace approve "+pendingID, &out)
	if console.pending != nil {
		t.Fatal("approved request not cleared")
	}
	if !strings.Contains(out.String(), "Local workspace grant created:") {
		t.Fatalf("approval not reported: %s", out.String())
	}
	// A concessão existe apenas no gerenciador local; nunca no endpoint HTTP.
	id := strings.Split(strings.Split(out.String(), "session=")[1], " ")[0]
	if _, err := console.grants.ReadText("other-owner", testConsoleClient, id, "file"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("other identity authorized: %v", err)
	}
	if text, err := console.grants.ReadText("local-owner", testConsoleClient, id, "file"); err != nil || text != "approved" {
		t.Fatalf("approved read: %q, %v", text, err)
	}
	if err := console.grants.ReplaceText("local-owner", testConsoleClient, id, "file", "approved", "must remain read-only"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("administrative workspace approval unexpectedly enabled writing: %v", err)
	}
	console.handleWorkspaceCommand("workspace revoke "+id, &out)
	if _, err := console.grants.ReadText("local-owner", testConsoleClient, id, "file"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("revoke failed: %v", err)
	}
}

func TestWorkspaceConsoleCancelsExpiresAndRejectsBroadRoots(t *testing.T) {
	console := testWorkspaceConsole(t)
	var out bytes.Buffer
	for _, root := range []string{"/", ".", "/tmp/../etc", "relative/path", strings.Repeat("a", 4097)} {
		console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+root, &out)
		if console.pending != nil {
			t.Fatalf("accepted invalid root %q", root)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+home, &out)
		if console.pending != nil {
			t.Fatal("accepted home")
		}
	}
	root := t.TempDir()
	console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+root, &out)
	id := console.pending.id
	console.handleWorkspaceCommand("workspace cancel "+id, &out)
	if console.pending != nil {
		t.Fatal("cancel retained pending grant")
	}
	console.handleWorkspaceCommand("workspace approve "+id, &out)
	console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+root, &out)
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
	console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+link, &out)
	id = console.pending.id
	console.handleWorkspaceCommand("workspace approve "+id, &out)
	if !strings.Contains(out.String(), "workspace grant rejected") {
		t.Fatal("symlink accepted")
	}
	if _, err := console.grants.ReadText("local-owner", testConsoleClient, id, "file"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("symlink root granted: %v", err)
	}
}

func TestWorkspaceConsoleShutdownRevokes(t *testing.T) {
	console := testWorkspaceConsole(t)
	root := t.TempDir()
	var out bytes.Buffer
	console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+root, &out)
	console.handleWorkspaceCommand("workspace approve "+console.pending.id, &out)
	if err := console.Close(); err != nil {
		t.Fatal(err)
	}
	console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+root, &out)
	console.handleWorkspaceCommand("workspace approve "+console.pending.id, &out)
	if !strings.Contains(out.String(), "workspace grant rejected: workspace session closed") {
		t.Fatalf("post-close grant accepted: %s", out.String())
	}
}

func TestWorkspaceConsoleRequiresIssuedClientAndDoesNotBindToChat(t *testing.T) {
	console := testWorkspaceConsole(t)
	root := filepath.Join(t.TempDir(), "pasta com espacos")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("shared"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	console.handleWorkspaceCommand("workspace clients", &out)
	if !strings.Contains(out.String(), testConsoleClient) || strings.Contains(out.String(), otherConsoleClient) {
		t.Fatalf("unexpected OAuth clients: %s", out.String())
	}
	console.handleWorkspaceCommand("workspace request "+otherConsoleClient+" "+root, &out)
	if console.pending != nil {
		t.Fatal("unissued OAuth client requested workspace")
	}
	console.handleWorkspaceCommand("workspace request "+root, &out)
	if console.pending != nil {
		t.Fatal("legacy root-only syntax accepted")
	}
	console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+root, &out)
	if console.pending == nil || console.pending.clientID != testConsoleClient || console.pending.root != root {
		t.Fatal("client binding or path with spaces lost")
	}
	console.handleWorkspaceCommand("workspace approve "+console.pending.id, &out)
	id := strings.Split(strings.Split(out.String(), "session=")[1], " ")[0]
	if _, err := console.grants.ReadText("local-owner", otherConsoleClient, id, "file"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("other client accessed workspace: %v", err)
	}
	// Chats diferentes usando o mesmo cliente passam pela mesma concessão.
	for i := 0; i < 2; i++ {
		if text, err := console.grants.ReadText("local-owner", testConsoleClient, id, "file"); err != nil || text != "shared" {
			t.Fatalf("authorized client denied: %q %v", text, err)
		}
	}
}

func TestWorkspaceConsoleProgrammingApprovalIsExplicitAndIndependent(t *testing.T) {
	grants, err := workspace.NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	approval, err := workspace.NewCapabilityApproval(grants, func(clientID string) bool { return clientID == testConsoleClient })
	if err != nil {
		_ = grants.Close()
		t.Fatal(err)
	}
	console := &workspaceConsole{
		grants:              grants,
		issuedClients:       func() []auth.ClientInfo { return []auth.ClientInfo{{ID: testConsoleClient, Name: "Cliente de teste"}} },
		programmingApproval: approval,
	}
	t.Cleanup(func() { _ = console.Close() })
	root := filepath.Join(t.TempDir(), "pasta com espacos")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// A composição padrão não injeta esta dependência experimental.
	defaultConsole := testWorkspaceConsole(t)
	defaultConsole.handleWorkspaceCommand("workspace request-programming "+testConsoleClient+" "+workspace.ScopeWrite+","+workspace.ScopeTest+" "+root, &out)
	if !strings.Contains(out.String(), "programming approval unavailable") {
		t.Fatalf("default console exposed programming approval: %s", out.String())
	}

	out.Reset()
	command := "workspace request-programming " + testConsoleClient + " " + workspace.ScopeWrite + "," + workspace.ScopeTest + "," + workspace.ScopeGit + " " + root
	console.handleWorkspaceCommand(command, &out)
	if !strings.Contains(out.String(), "Cliente de teste") || !strings.Contains(out.String(), "go test ./...") || !strings.Contains(out.String(), "sem commit/push") {
		t.Fatalf("programming consent summary is incomplete: %s", out.String())
	}
	approvalID := regexp.MustCompile(`workspace approve-programming ([a-f0-9]{32})`).FindStringSubmatch(out.String())
	if len(approvalID) != 2 {
		t.Fatalf("missing programming approval identifier: %s", out.String())
	}
	if grants.AllowsClientScope(testConsoleClient, workspace.ScopeWrite) || grants.AllowsClientScope(testConsoleClient, workspace.ScopeTest) || grants.AllowsClientScope(testConsoleClient, workspace.ScopeGit) {
		t.Fatal("programming request created a grant before confirmation")
	}
	console.handleWorkspaceCommand("workspace approve-programming wrong", &out)
	if !strings.Contains(out.String(), "approval missing") {
		t.Fatalf("invalid programming approval was not rejected: %s", out.String())
	}
	console.handleWorkspaceCommand("workspace approve-programming "+approvalID[1], &out)
	match := regexp.MustCompile(`Local programming workspace grant created: session=([a-f0-9]{32})`).FindStringSubmatch(out.String())
	if len(match) != 2 {
		t.Fatalf("programming grant was not created after confirmation: %s", out.String())
	}
	if !grants.AllowsClientScope(testConsoleClient, workspace.ScopeWrite) || !grants.AllowsClientScope(testConsoleClient, workspace.ScopeTest) || !grants.AllowsClientScope(testConsoleClient, workspace.ScopeGit) || grants.AllowsClientScope(testConsoleClient, workspace.ScopeRead) {
		t.Fatal("programming grant changed unselected capabilities")
	}
	console.handleWorkspaceCommand("workspace revoke "+match[1], &out)
	if grants.AllowsClientScope(testConsoleClient, workspace.ScopeWrite) || grants.AllowsClientScope(testConsoleClient, workspace.ScopeTest) || grants.AllowsClientScope(testConsoleClient, workspace.ScopeGit) {
		t.Fatal("revocation retained programming capabilities")
	}
}
