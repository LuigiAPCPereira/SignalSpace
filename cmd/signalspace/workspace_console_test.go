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

func TestWorkspaceConsoleManagedWorktreeLifecycleUsesSeparateApproval(t *testing.T) {
	console := testWorkspaceConsole(t)
	manager, err := workspace.NewManagedWorktreeManager(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	console.managed = manager
	root := gitWorkspaceFixture(t)
	var out bytes.Buffer
	command := "workspace request-worktree " + testConsoleClient + " " + workspace.ScopeRead + "," + workspace.ScopeWrite + "," + workspace.ScopeGitIndex + " " + root
	if !console.handleWorkspaceCommand(command, &out) || console.managedPending == nil {
		t.Fatalf("managed request was not pending: %s", out.String())
	}
	approvalID := console.managedPending.id
	console.handleWorkspaceCommand("workspace approve-worktree "+approvalID, &out)
	snapshot, err := console.grants.Snapshot()
	if err != nil || !snapshot.Active || snapshot.Mode != workspace.WorkspaceModeWorktree || snapshot.ManagedWorkspaceID == "" || !containsScope(snapshot.Scopes, workspace.ScopeGitIndex) {
		t.Fatalf("managed grant snapshot: %#v %v\n%s", snapshot, err, out.String())
	}
	oldSession := snapshot.SessionID
	if text, err := console.grants.ReadText("local-owner", testConsoleClient, oldSession, "tracked.txt"); err != nil || text != "before\n" {
		t.Fatalf("managed console read: %q %v", text, err)
	}
	console.handleWorkspaceCommand("workspace revoke "+oldSession, &out)
	if console.managed.ActiveID() != "" {
		t.Fatal("revoke left managed workspace active")
	}
	console.handleWorkspaceCommand("workspace request-worktree-resume "+testConsoleClient+" "+workspace.ScopeRead+","+workspace.ScopeGitIndex+" "+snapshot.ManagedWorkspaceID, &out)
	if console.managedPending == nil {
		t.Fatalf("resume request was not pending: %s", out.String())
	}
	console.handleWorkspaceCommand("workspace approve-worktree-resume "+console.managedPending.id, &out)
	resumed, err := console.grants.Snapshot()
	if err != nil || !resumed.Active || resumed.SessionID == oldSession || resumed.ManagedWorkspaceID != snapshot.ManagedWorkspaceID || !containsScope(resumed.Scopes, workspace.ScopeGitIndex) {
		t.Fatalf("managed resume snapshot: %#v %v\n%s", resumed, err, out.String())
	}
	console.handleWorkspaceCommand("workspace revoke "+resumed.SessionID, &out)
	console.handleWorkspaceCommand("workspace remove-worktree "+snapshot.ManagedWorkspaceID, &out)
	if _, err := console.managed.Descriptor(snapshot.ManagedWorkspaceID); !errors.Is(err, workspace.ErrManagedWorkspaceNotFound) {
		t.Fatalf("managed worktree was not removed: %v\n%s", err, out.String())
	}
}

func TestWorkspaceConsoleCanonicalManagedProgrammingAppliesStandardProfile(t *testing.T) {
	console := testWorkspaceConsole(t)
	manager, err := workspace.NewManagedWorktreeManager(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	console.managed = manager
	if err := manager.SetGitIdentity("owner@example.invalid", "Owner Local"); err != nil {
		t.Fatal(err)
	}
	var applied workspace.GrantSnapshot
	console.programmingProfile = func(snapshot workspace.GrantSnapshot) error {
		applied = snapshot
		return nil
	}
	root := gitWorkspaceFixture(t)
	var out bytes.Buffer
	console.handleWorkspaceCommand("workspace request-worktree "+testConsoleClient+" "+root, &out)
	if console.managedPending == nil || !strings.Contains(out.String(), "Managed Programming solicitada") {
		t.Fatalf("canonical managed Programming request was not created: %s", out.String())
	}
	console.handleWorkspaceCommand("workspace approve-worktree "+console.managedPending.id, &out)
	if applied.ManagedWorkspaceID == "" || applied.Mode != workspace.WorkspaceModeWorktree || !applied.Active {
		t.Fatalf("managed Standard profile was not applied: %+v\n%s", applied, out.String())
	}
	for _, wanted := range []string{workspace.ScopeRead, workspace.ScopeWrite, workspace.ScopeGit, workspace.ScopeGitIndex, workspace.ScopeGitCommit} {
		if !containsScope(applied.Scopes, wanted) {
			t.Fatalf("managed Standard envelope omitted %s: %+v", wanted, applied)
		}
	}
	if !strings.Contains(out.String(), "Managed Programming grant created") || !strings.Contains(out.String(), "profile=STANDARD") {
		t.Fatalf("managed Standard activation was not reported: %s", out.String())
	}
	console.handleWorkspaceCommand("workspace revoke "+applied.SessionID, &out)
	console.handleWorkspaceCommand("workspace remove-worktree "+applied.ManagedWorkspaceID, &out)
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
	console.handleWorkspaceCommand("workspace request-programming "+testConsoleClient+" "+workspace.ScopeGitIndex+" "+root, &out)
	if console.pending != nil || !strings.Contains(out.String(), "programming request rejected") {
		t.Fatalf("generic programming request accepted managed-only Git index: %s", out.String())
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
	out.Reset()
	console.handleWorkspaceCommand("workspace status", &out)
	for _, expected := range []string{workspace.ScopeWrite, workspace.ScopeTest, workspace.ScopeGit, "não estão ativados remotamente"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("experimental status omitted %q: %s", expected, out.String())
		}
	}
	console.handleWorkspaceCommand("workspace revoke "+match[1], &out)
	if grants.AllowsClientScope(testConsoleClient, workspace.ScopeWrite) || grants.AllowsClientScope(testConsoleClient, workspace.ScopeTest) || grants.AllowsClientScope(testConsoleClient, workspace.ScopeGit) {
		t.Fatal("revocation retained programming capabilities")
	}
}

func TestWorkspaceConsoleCanonicalProgrammingAppliesStandardProfileAfterGrant(t *testing.T) {
	grants, err := workspace.NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	approval, err := workspace.NewCapabilityApproval(grants, func(clientID string) bool { return clientID == testConsoleClient })
	if err != nil {
		_ = grants.Close()
		t.Fatal(err)
	}
	var applied workspace.GrantSnapshot
	console := &workspaceConsole{
		grants:              grants,
		issuedClients:       func() []auth.ClientInfo { return []auth.ClientInfo{{ID: testConsoleClient, Name: "Cliente de teste"}} },
		programmingApproval: approval,
		programmingProfile: func(snapshot workspace.GrantSnapshot) error {
			applied = snapshot
			return nil
		},
	}
	t.Cleanup(func() { _ = console.Close() })
	root := filepath.Join(t.TempDir(), "checkout")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	console.handleWorkspaceCommand("workspace request-programming "+testConsoleClient+" "+root, &out)
	approvalID := regexp.MustCompile(`workspace approve-programming ([a-f0-9]{32})`).FindStringSubmatch(out.String())
	if len(approvalID) != 2 || !strings.Contains(out.String(), "profile STANDARD") {
		t.Fatalf("canonical Programming request was not created: %s", out.String())
	}
	console.handleWorkspaceCommand("workspace approve-programming "+approvalID[1], &out)
	if applied.SessionID == "" || !applied.Active || applied.Mode != workspace.WorkspaceModeCheckout {
		t.Fatalf("profile was not applied after grant: %+v\n%s", applied, out.String())
	}
	if !strings.Contains(out.String(), "Local Programming Standard grant created") {
		t.Fatalf("standard activation was not reported: %s", out.String())
	}
	if !containsScope(applied.Scopes, workspace.ScopeRead) || !containsScope(applied.Scopes, workspace.ScopeWrite) || !containsScope(applied.Scopes, workspace.ScopeGit) {
		t.Fatalf("standard checkout envelope is incomplete: %+v", applied)
	}
}

func TestWorkspaceConsoleStatusIsLocalAndDoesNotExposeWorkspaceData(t *testing.T) {
	console := testWorkspaceConsole(t)
	root := t.TempDir()
	const privateContents = "private marker not for status"
	if err := os.WriteFile(filepath.Join(root, "file"), []byte(privateContents), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := console.grants.GrantWithScopes(root, testConsoleClient, workspace.ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if !console.handleWorkspaceCommand("workspace status", &out) {
		t.Fatal("status command was not handled")
	}
	for _, expected := range []string{id, testConsoleClient, "Cliente de teste", workspace.ScopeRead, "não comprova token OAuth", "não publica read_file", "connection_diagnostic"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("status omitted %q: %s", expected, out.String())
		}
	}
	if strings.Contains(out.String(), root) || strings.Contains(out.String(), privateContents) {
		t.Fatalf("status exposed workspace data: %s", out.String())
	}

	// A ausência do cliente na lista atual só remove o nome; ela não transforma
	// nem descreve a concessão como revogada.
	console.issuedClients = func() []auth.ClientInfo { return nil }
	out.Reset()
	console.handleWorkspaceCommand("workspace status", &out)
	if !strings.Contains(out.String(), "Nome declarado: indisponível") || !strings.Contains(out.String(), "não permite inferir revogação") {
		t.Fatalf("missing client name was misrepresented: %s", out.String())
	}
	current, err := console.grants.Snapshot()
	if err != nil || !current.Active || current.SessionID != id {
		t.Fatalf("status changed the active grant: %+v, %v", current, err)
	}
}

func TestWorkspaceConsoleStatusAndRevokeCurrentStates(t *testing.T) {
	console := testWorkspaceConsole(t)
	var out bytes.Buffer
	console.handleWorkspaceCommand("workspace status", &out)
	if !strings.Contains(out.String(), "Concessão local: ausente") {
		t.Fatalf("empty status: %s", out.String())
	}

	root := t.TempDir()
	console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+root, &out)
	out.Reset()
	console.handleWorkspaceCommand("workspace status", &out)
	if !strings.Contains(out.String(), "Concessão local: ausente") || !strings.Contains(out.String(), "não são concessões") {
		t.Fatalf("pending approval appeared as a grant: %s", out.String())
	}
	console.handleWorkspaceCommand("workspace cancel "+console.pending.id, &out)
	out.Reset()
	console.handleWorkspaceCommand("workspace status", &out)
	if !strings.Contains(out.String(), "Concessão local: ausente") {
		t.Fatalf("canceled approval changed grant status: %s", out.String())
	}

	console.handleWorkspaceCommand("workspace request "+testConsoleClient+" "+root, &out)
	console.pending.expires = time.Now().Add(-time.Second)
	console.handleWorkspaceCommand("workspace approve "+console.pending.id, &out)
	out.Reset()
	console.handleWorkspaceCommand("workspace status", &out)
	if !strings.Contains(out.String(), "Concessão local: ausente") {
		t.Fatalf("expired approval changed grant status: %s", out.String())
	}

	firstID, err := console.grants.Grant(root, testConsoleClient)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	console.handleWorkspaceCommand("workspace status extra", &out)
	console.handleWorkspaceCommand("workspace revoke current extra", &out)
	if !strings.Contains(out.String(), "use workspace status") || !strings.Contains(out.String(), "use workspace revoke") {
		t.Fatalf("extra arguments were not rejected: %s", out.String())
	}
	current, err := console.grants.Snapshot()
	if err != nil || !current.Active || current.SessionID != firstID {
		t.Fatalf("invalid commands changed grant: %+v, %v", current, err)
	}

	out.Reset()
	console.handleWorkspaceCommand("workspace revoke current", &out)
	if !strings.Contains(out.String(), "workspace grant revoked") {
		t.Fatalf("current grant revoke failed: %s", out.String())
	}
	current, err = console.grants.Snapshot()
	if err != nil || current.Active {
		t.Fatalf("grant remains after revoke current: %+v, %v", current, err)
	}
	console.handleWorkspaceCommand("workspace revoke current", &out)
	if !strings.Contains(out.String(), "no active workspace grant") {
		t.Fatalf("missing current grant reported incorrectly: %s", out.String())
	}

	legacyID, err := console.grants.Grant(root, testConsoleClient)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	console.handleWorkspaceCommand("workspace revoke "+legacyID, &out)
	if !strings.Contains(out.String(), "workspace grant revoked") {
		t.Fatalf("legacy session-ID revoke failed: %s", out.String())
	}

	if err := console.Close(); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	console.handleWorkspaceCommand("workspace status", &out)
	if !strings.Contains(out.String(), "indisponível (instância encerrada)") || strings.Contains(out.String(), "Concessão local: ausente") {
		t.Fatalf("closed grants were confused with absence: %s", out.String())
	}
}
