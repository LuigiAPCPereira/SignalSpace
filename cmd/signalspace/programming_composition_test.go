package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func TestPublicProgrammingCompositionPromotesOnlyReadWriteAndGit(t *testing.T) {
	handler, authorization, console, err := embeddedHandlerWithWorkspace(readTestResource, filepath.Join(t.TempDir(), "identity"), compositionProgramming)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = console.Close()
		_ = authorization.Close()
	})

	metadata := readRequest(t, handler, http.MethodGet, "/.well-known/oauth-authorization-server", "", "", "", nil)
	if metadata.Code != http.StatusOK {
		t.Fatalf("metadata: %d %s", metadata.Code, metadata.Body.String())
	}
	var metadataPayload struct {
		Scopes []string `json:"scopes_supported"`
	}
	if err := json.Unmarshal(metadata.Body.Bytes(), &metadataPayload); err != nil {
		t.Fatal(err)
	}
	wantScopes := []string{compositionDiagnosticScope, workspace.ScopeRead, workspace.ScopeWrite, workspace.ScopeGit}
	if strings.Join(metadataPayload.Scopes, " ") != strings.Join(wantScopes, " ") {
		t.Fatalf("unexpected programming metadata scopes: %v", metadataPayload.Scopes)
	}

	registration := fmt.Sprintf(`{"client_name":"Public programming test","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	registered := readRequest(t, handler, http.MethodPost, "/register", registration, "application/json", "", nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", registered.Code, registered.Body.String())
	}
	var client struct {
		ID string `json:"client_id"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &client); err != nil || client.ID == "" {
		t.Fatalf("client registration: %v", err)
	}

	// O token diagnóstico torna o cliente elegível, mas não concede nenhuma
	// capacidade de workspace por si só.
	diagnosticToken := authorizeClient(t, handler, func(id string) error {
		return authorization.DecideTerminal(id, true)
	}, client.ID, compositionDiagnosticScope)
	assertPublicToolNames(t, handler, diagnosticToken, "connection_diagnostic")

	root := gitWorkspaceFixture(t)
	var approvalOutput strings.Builder
	console.handleWorkspaceCommand("workspace request-programming "+client.ID+" "+strings.Join([]string{workspace.ScopeRead, workspace.ScopeWrite, workspace.ScopeGit}, ",")+" "+root, &approvalOutput)
	approvalID := regexp.MustCompile(`workspace approve-programming ([a-f0-9]{32})`).FindStringSubmatch(approvalOutput.String())
	if len(approvalID) != 2 {
		t.Fatalf("missing programming approval: %s", approvalOutput.String())
	}
	console.handleWorkspaceCommand("workspace approve-programming "+approvalID[1], &approvalOutput)
	sessionMatch := regexp.MustCompile(`Local programming workspace grant created: session=([a-f0-9]{32})`).FindStringSubmatch(approvalOutput.String())
	if len(sessionMatch) != 2 {
		t.Fatalf("programming grant was not created: %s", approvalOutput.String())
	}
	sessionID := sessionMatch[1]

	readToken := authorizeClient(t, handler, func(id string) error { return authorization.DecideTerminal(id, true) }, client.ID, compositionDiagnosticScope+" "+workspace.ScopeRead)
	writeToken := authorizeClient(t, handler, func(id string) error { return authorization.DecideTerminal(id, true) }, client.ID, compositionDiagnosticScope+" "+workspace.ScopeWrite)
	gitToken := authorizeClient(t, handler, func(id string) error { return authorization.DecideTerminal(id, true) }, client.ID, compositionDiagnosticScope+" "+workspace.ScopeGit)
	allToken := authorizeClient(t, handler, func(id string) error { return authorization.DecideTerminal(id, true) }, client.ID, compositionDiagnosticScope+" "+workspace.ScopeRead+" "+workspace.ScopeWrite+" "+workspace.ScopeGit)

	assertPublicToolNames(t, handler, readToken, "connection_diagnostic", "read_file", "list_directory", "stat_path", "find_paths", "search_text")
	assertPublicToolNames(t, handler, writeToken, "connection_diagnostic", "replace_text", "create_directory", "create_text_file", "write_text_file")
	assertPublicToolNames(t, handler, gitToken, "connection_diagnostic", "review_git_changes")
	assertPublicToolNames(t, handler, allToken, "connection_diagnostic", "read_file", "list_directory", "stat_path", "find_paths", "search_text", "replace_text", "create_directory", "create_text_file", "write_text_file", "review_git_changes")
	for _, token := range []string{readToken, writeToken, gitToken, allToken} {
		listing := readRequest(t, handler, http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "application/json", token, nil)
		if strings.Contains(listing.Body.String(), `"name":"run_workspace_tests"`) || strings.Contains(listing.Body.String(), `"name":"shell"`) {
			t.Fatalf("programming surface exposed forbidden capability: %s", listing.Body.String())
		}
	}

	assertPublicInitializeInstructions(t, handler, readToken, "File reading and directory listing require a separate OAuth read scope, an active local workspace grant and its session ID. No editing or commands. Structured path metadata, bounded path search and literal text search are available without shell.")
	assertPublicInitializeInstructions(t, handler, writeToken, "Workspace text replacement requires a separate OAuth write scope, an active local workspace write grant and its session ID. No commands or Git mutations. Directory creation, create-only text files and hash-preconditioned full-file updates use the same separate write scope.")

	readCall := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"session_id":"` + sessionID + `","path":"tracked.txt"}}}`
	readResponse := readRequest(t, handler, http.MethodPost, "/mcp", readCall, "application/json", readToken, nil)
	if readResponse.Code != http.StatusOK || !strings.Contains(readResponse.Body.String(), `before\n`) {
		t.Fatalf("authorized public read failed: %d %s", readResponse.Code, readResponse.Body.String())
	}
	statCall := `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"stat_path","arguments":{"session_id":"` + sessionID + `","path":"tracked.txt"}}}`
	statResponse := readRequest(t, handler, http.MethodPost, "/mcp", statCall, "application/json", readToken, nil)
	if statResponse.Code != http.StatusOK || !strings.Contains(statResponse.Body.String(), `"kind":"regular_file"`) || strings.Contains(statResponse.Body.String(), root) {
		t.Fatalf("authorized public stat failed or leaked root: %d %s", statResponse.Code, statResponse.Body.String())
	}
	findCall := `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"find_paths","arguments":{"session_id":"` + sessionID + `","pattern":"*.txt"}}}`
	findResponse := readRequest(t, handler, http.MethodPost, "/mcp", findCall, "application/json", readToken, nil)
	if findResponse.Code != http.StatusOK || !strings.Contains(findResponse.Body.String(), "tracked.txt") {
		t.Fatalf("authorized public find failed: %d %s", findResponse.Code, findResponse.Body.String())
	}
	searchCall := `{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"search_text","arguments":{"session_id":"` + sessionID + `","query":"before"}}}`
	searchResponse := readRequest(t, handler, http.MethodPost, "/mcp", searchCall, "application/json", readToken, nil)
	if searchResponse.Code != http.StatusOK || !strings.Contains(searchResponse.Body.String(), "tracked.txt") {
		t.Fatalf("authorized public search failed: %d %s", searchResponse.Code, searchResponse.Body.String())
	}
	createDirectoryCall := `{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"create_directory","arguments":{"session_id":"` + sessionID + `","path":"generated"}}}`
	createDirectoryResponse := readRequest(t, handler, http.MethodPost, "/mcp", createDirectoryCall, "application/json", writeToken, nil)
	if createDirectoryResponse.Code != http.StatusOK || !strings.Contains(createDirectoryResponse.Body.String(), `"status":"created"`) {
		t.Fatalf("authorized public directory creation failed: %d %s", createDirectoryResponse.Code, createDirectoryResponse.Body.String())
	}
	createFileCall := `{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"create_text_file","arguments":{"session_id":"` + sessionID + `","path":"generated/file.txt","content":"before"}}}`
	createFileResponse := readRequest(t, handler, http.MethodPost, "/mcp", createFileCall, "application/json", writeToken, nil)
	if createFileResponse.Code != http.StatusOK || !strings.Contains(createFileResponse.Body.String(), `"status":"created"`) {
		t.Fatalf("authorized public file creation failed: %d %s", createFileResponse.Code, createFileResponse.Body.String())
	}
	fileSum := sha256.Sum256([]byte("before"))
	writeFileCall := `{"jsonrpc":"2.0","id":16,"method":"tools/call","params":{"name":"write_text_file","arguments":{"session_id":"` + sessionID + `","path":"generated/file.txt","expected_sha256":"` + hex.EncodeToString(fileSum[:]) + `","content":"after"}}}`
	writeFileResponse := readRequest(t, handler, http.MethodPost, "/mcp", writeFileCall, "application/json", writeToken, nil)
	if writeFileResponse.Code != http.StatusOK || !strings.Contains(writeFileResponse.Body.String(), `"status":"updated"`) {
		t.Fatalf("authorized public full-file update failed: %d %s", writeFileResponse.Code, writeFileResponse.Body.String())
	}

	writeCall := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"replace_text","arguments":{"session_id":"` + sessionID + `","path":"tracked.txt","expected":"before\n","replacement":"after\n"}}}`
	writeResponse := readRequest(t, handler, http.MethodPost, "/mcp", writeCall, "application/json", writeToken, nil)
	if writeResponse.Code != http.StatusOK || !strings.Contains(writeResponse.Body.String(), "Workspace text replaced.") {
		t.Fatalf("authorized public write failed: %d %s", writeResponse.Code, writeResponse.Body.String())
	}

	gitCall := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"review_git_changes","arguments":{"session_id":"` + sessionID + `"}}}`
	gitResponse := readRequest(t, handler, http.MethodPost, "/mcp", gitCall, "application/json", gitToken, nil)
	if gitResponse.Code != http.StatusOK || !strings.Contains(gitResponse.Body.String(), "diff_changed") || !strings.Contains(gitResponse.Body.String(), "+after") {
		t.Fatalf("authorized public Git review failed: %d %s", gitResponse.Code, gitResponse.Body.String())
	}

	missingGitScope := readRequest(t, handler, http.MethodPost, "/mcp", gitCall, "application/json", writeToken, nil)
	if !strings.Contains(missingGitScope.Body.String(), "Git review authorization required") {
		t.Fatalf("write bearer reached Git without Git scope: %s", missingGitScope.Body.String())
	}
	missingWriteScope := readRequest(t, handler, http.MethodPost, "/mcp", writeCall, "application/json", readToken, nil)
	if !strings.Contains(missingWriteScope.Body.String(), "Workspace write authorization required") {
		t.Fatalf("read bearer reached writer without write scope: %s", missingWriteScope.Body.String())
	}

	if err := console.grants.Revoke(sessionID); err != nil {
		t.Fatal(err)
	}
	revoked := readRequest(t, handler, http.MethodPost, "/mcp", writeCall, "application/json", writeToken, nil)
	if !strings.Contains(revoked.Body.String(), "Workspace write unavailable or not authorized") {
		t.Fatalf("revoked JWT retained write capability: %s", revoked.Body.String())
	}
}

func gitWorkspaceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, output)
		}
	}
	run("init", "--quiet")
	run("config", "user.email", "signalspace-test@example.invalid")
	run("config", "user.name", "SignalSpace test")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", "tracked.txt")
	run("commit", "--quiet", "-m", "baseline")
	return root
}
