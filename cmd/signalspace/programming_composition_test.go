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

func TestPublicProgrammingCompositionPromotesOnlyReadWriteGitAndManagedIndex(t *testing.T) {
	programmingTools := []string{"connection_diagnostic", "read_file", "list_directory", "stat_path", "find_paths", "search_text", "replace_text", "create_directory", "create_text_file", "write_text_file", "copy_path", "move_path", "delete_file", "delete_directory", "apply_patch", "review_git_changes", "git_status", "stage_git_paths", "unstage_git_paths", "commit_git_index"}
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
	wantScopes := []string{compositionDiagnosticScope, workspace.ScopeRead, workspace.ScopeWrite, workspace.ScopeGit, workspace.ScopeGitIndex, workspace.ScopeGitCommit}
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

	// O token diagnóstico torna o cliente elegível, mas a descoberta da
	// composição programming não depende de scopes ou grants locais.
	diagnosticToken := authorizeClient(t, handler, func(id string) error {
		return authorization.DecideTerminal(id, true)
	}, client.ID, compositionDiagnosticScope)
	assertPublicToolNames(t, handler, diagnosticToken, programmingTools...)
	assertProgrammingSecuritySchemes(t, handler, diagnosticToken)
	assertPublicInitializeInstructionsContains(t, handler, diagnosticToken,
		"Programming tools are discoverable",
		"separate OAuth scope",
		"active local grant",
		"managed SignalSpace worktree",
		"no shell")

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

	assertPublicToolNames(t, handler, readToken, programmingTools...)
	assertPublicToolNames(t, handler, writeToken, programmingTools...)
	assertPublicToolNames(t, handler, gitToken, programmingTools...)
	assertPublicToolNames(t, handler, allToken, programmingTools...)
	for _, token := range []string{readToken, writeToken, gitToken, allToken} {
		listing := readRequest(t, handler, http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "application/json", token, nil)
		if strings.Contains(listing.Body.String(), `"name":"run_workspace_tests"`) || strings.Contains(listing.Body.String(), `"name":"shell"`) {
			t.Fatalf("programming surface exposed forbidden capability: %s", listing.Body.String())
		}
	}

	assertProgrammingInsufficientScopeHasNoEffects(t, handler, diagnosticToken, root)

	assertPublicInitializeInstructionsContains(t, handler, readToken,
		"Programming tools are discoverable", "separate OAuth scope", "active local grant", "managed SignalSpace worktree", "no shell")
	assertPublicInitializeInstructionsContains(t, handler, writeToken,
		"Programming tools are discoverable", "separate OAuth scope", "active local grant", "managed SignalSpace worktree", "no shell")

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
	copyCall := `{"jsonrpc":"2.0","id":17,"method":"tools/call","params":{"name":"copy_path","arguments":{"session_id":"` + sessionID + `","source":"tracked.txt","destination":"copied.txt"}}}`
	copyResponse := readRequest(t, handler, http.MethodPost, "/mcp", copyCall, "application/json", writeToken, nil)
	if copyResponse.Code != http.StatusOK || !strings.Contains(copyResponse.Body.String(), `"status":"copied"`) {
		t.Fatalf("authorized public copy failed: %d %s", copyResponse.Code, copyResponse.Body.String())
	}
	moveCall := `{"jsonrpc":"2.0","id":18,"method":"tools/call","params":{"name":"move_path","arguments":{"session_id":"` + sessionID + `","source":"copied.txt","destination":"moved.txt"}}}`
	moveResponse := readRequest(t, handler, http.MethodPost, "/mcp", moveCall, "application/json", writeToken, nil)
	if moveResponse.Code != http.StatusOK || !strings.Contains(moveResponse.Body.String(), `"status":"moved"`) {
		t.Fatalf("authorized public move failed: %d %s", moveResponse.Code, moveResponse.Body.String())
	}
	deleteFileCall := `{"jsonrpc":"2.0","id":19,"method":"tools/call","params":{"name":"delete_file","arguments":{"session_id":"` + sessionID + `","path":"moved.txt"}}}`
	deleteFileResponse := readRequest(t, handler, http.MethodPost, "/mcp", deleteFileCall, "application/json", writeToken, nil)
	if deleteFileResponse.Code != http.StatusOK || !strings.Contains(deleteFileResponse.Body.String(), `"status":"deleted"`) {
		t.Fatalf("authorized public file deletion failed: %d %s", deleteFileResponse.Code, deleteFileResponse.Body.String())
	}
	emptyDirectoryCall := `{"jsonrpc":"2.0","id":20,"method":"tools/call","params":{"name":"create_directory","arguments":{"session_id":"` + sessionID + `","path":"empty"}}}`
	if response := readRequest(t, handler, http.MethodPost, "/mcp", emptyDirectoryCall, "application/json", writeToken, nil); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"created"`) {
		t.Fatalf("empty directory setup failed: %d %s", response.Code, response.Body.String())
	}
	deleteDirectoryCall := `{"jsonrpc":"2.0","id":21,"method":"tools/call","params":{"name":"delete_directory","arguments":{"session_id":"` + sessionID + `","path":"empty"}}}`
	deleteDirectoryResponse := readRequest(t, handler, http.MethodPost, "/mcp", deleteDirectoryCall, "application/json", writeToken, nil)
	if deleteDirectoryResponse.Code != http.StatusOK || !strings.Contains(deleteDirectoryResponse.Body.String(), `"status":"deleted"`) {
		t.Fatalf("authorized public directory deletion failed: %d %s", deleteDirectoryResponse.Code, deleteDirectoryResponse.Body.String())
	}

	gitCall := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"review_git_changes","arguments":{"session_id":"` + sessionID + `"}}}`
	gitResponse := readRequest(t, handler, http.MethodPost, "/mcp", gitCall, "application/json", gitToken, nil)
	if gitResponse.Code != http.StatusOK || !strings.Contains(gitResponse.Body.String(), "diff_changed") || !strings.Contains(gitResponse.Body.String(), "+after") {
		t.Fatalf("authorized public Git review failed: %d %s", gitResponse.Code, gitResponse.Body.String())
	}
	statusCall := `{"jsonrpc":"2.0","id":22,"method":"tools/call","params":{"name":"git_status","arguments":{"session_id":"` + sessionID + `"}}}`
	statusResponse := readRequest(t, handler, http.MethodPost, "/mcp", statusCall, "application/json", gitToken, nil)
	if statusResponse.Code != http.StatusOK || !strings.Contains(statusResponse.Body.String(), `\"index_sha256\"`) {
		t.Fatalf("authorized public Git status failed: %d %s", statusResponse.Code, statusResponse.Body.String())
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

func assertPublicInitializeInstructionsContains(t *testing.T, handler http.Handler, token string, fragments ...string) {
	t.Helper()
	initialized := readRequest(t, handler, http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`, "application/json", token, nil)
	if initialized.Code != http.StatusOK {
		t.Fatalf("initialize: %d %s", initialized.Code, initialized.Body.String())
	}
	var payload struct {
		Result struct {
			Instructions string `json:"instructions"`
		} `json:"result"`
	}
	if err := json.Unmarshal(initialized.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range fragments {
		if !strings.Contains(payload.Result.Instructions, fragment) {
			t.Fatalf("initialize instructions omitted %q: %q", fragment, payload.Result.Instructions)
		}
	}
}

func assertProgrammingSecuritySchemes(t *testing.T, handler http.Handler, token string) {
	t.Helper()
	listing := readRequest(t, handler, http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "application/json", token, nil)
	if listing.Code != http.StatusOK {
		t.Fatalf("tools/list: %d %s", listing.Code, listing.Body.String())
	}
	var payload struct {
		Result struct {
			Tools []struct {
				Name            string `json:"name"`
				SecuritySchemes []struct {
					Type   string   `json:"type"`
					Scopes []string `json:"scopes"`
				} `json:"securitySchemes"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(listing.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"connection_diagnostic": {compositionDiagnosticScope},
		"read_file":             {compositionDiagnosticScope, workspace.ScopeRead},
		"list_directory":        {compositionDiagnosticScope, workspace.ScopeRead},
		"stat_path":             {compositionDiagnosticScope, workspace.ScopeRead},
		"find_paths":            {compositionDiagnosticScope, workspace.ScopeRead},
		"search_text":           {compositionDiagnosticScope, workspace.ScopeRead},
		"replace_text":          {compositionDiagnosticScope, workspace.ScopeWrite},
		"create_directory":      {compositionDiagnosticScope, workspace.ScopeWrite},
		"create_text_file":      {compositionDiagnosticScope, workspace.ScopeWrite},
		"write_text_file":       {compositionDiagnosticScope, workspace.ScopeWrite},
		"copy_path":             {compositionDiagnosticScope, workspace.ScopeWrite},
		"move_path":             {compositionDiagnosticScope, workspace.ScopeWrite},
		"delete_file":           {compositionDiagnosticScope, workspace.ScopeWrite},
		"delete_directory":      {compositionDiagnosticScope, workspace.ScopeWrite},
		"apply_patch":           {compositionDiagnosticScope, workspace.ScopeWrite},
		"review_git_changes":    {compositionDiagnosticScope, workspace.ScopeGit},
		"git_status":            {compositionDiagnosticScope, workspace.ScopeGit},
		"stage_git_paths":       {compositionDiagnosticScope, workspace.ScopeGitIndex},
		"unstage_git_paths":     {compositionDiagnosticScope, workspace.ScopeGitIndex},
		"commit_git_index":      {compositionDiagnosticScope, workspace.ScopeGitCommit},
	}
	for _, tool := range payload.Result.Tools {
		if tool.Name == "run_workspace_tests" || tool.Name == "shell" {
			t.Fatalf("forbidden tool discovered: %s", tool.Name)
		}
		wantScopes, ok := want[tool.Name]
		if !ok {
			t.Fatalf("unexpected programming tool: %s", tool.Name)
		}
		if len(tool.SecuritySchemes) != 1 || tool.SecuritySchemes[0].Type != "oauth2" || strings.Join(tool.SecuritySchemes[0].Scopes, " ") != strings.Join(wantScopes, " ") {
			t.Fatalf("unexpected security schemes for %s: %+v", tool.Name, tool.SecuritySchemes)
		}
	}
	if len(payload.Result.Tools) != len(want) {
		t.Fatalf("programming discovery omitted tools: got=%d want=%d", len(payload.Result.Tools), len(want))
	}
}

func assertProgrammingInsufficientScopeHasNoEffects(t *testing.T, handler http.Handler, token, root string) {
	t.Helper()
	beforeBytes, err := os.ReadFile(filepath.Join(root, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	beforeHead := gitOutput(t, root, "rev-parse", "HEAD")
	beforeCached := gitOutput(t, root, "diff", "--cached", "--raw")
	fileHash := sha256.Sum256(beforeBytes)
	for _, testCase := range []struct {
		name  string
		scope string
		args  map[string]any
	}{
		{name: "read", scope: workspace.ScopeRead, args: map[string]any{"session_id": "missing-scope", "path": "tracked.txt"}},
		{name: "write", scope: workspace.ScopeWrite, args: map[string]any{"session_id": "missing-scope", "path": "tracked.txt", "expected": string(beforeBytes), "replacement": "must-not-write"}},
		{name: "git-review", scope: workspace.ScopeGit, args: map[string]any{"session_id": "missing-scope"}},
		{name: "git-index", scope: workspace.ScopeGitIndex, args: map[string]any{"session_id": "missing-scope", "expected_index_sha256": strings.Repeat("0", 64), "entries": []map[string]any{{"path": "tracked.txt", "expected_sha256": hex.EncodeToString(fileHash[:])}}}},
		{name: "git-commit", scope: workspace.ScopeGitCommit, args: map[string]any{"session_id": "missing-scope", "expected_head_oid": strings.Repeat("0", 40), "expected_index_sha256": strings.Repeat("0", 64), "message": "must-not-commit"}},
	} {
		name := map[string]string{"read": "read_file", "write": "replace_text", "git-review": "review_git_changes", "git-index": "stage_git_paths", "git-commit": "commit_git_index"}[testCase.name]
		params, err := json.Marshal(map[string]any{"name": name, "arguments": testCase.args})
		if err != nil {
			t.Fatal(err)
		}
		request, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 100, "method": "tools/call", "params": json.RawMessage(params)})
		if err != nil {
			t.Fatal(err)
		}
		response := readRequest(t, handler, http.MethodPost, "/mcp", string(request), "application/json", token, nil)
		var payload struct {
			Result struct {
				IsError bool `json:"isError"`
				Meta    struct {
					Authenticate []string `json:"mcp/www_authenticate"`
				} `json:"_meta"`
			} `json:"result"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &payload) != nil || !payload.Result.IsError || len(payload.Result.Meta.Authenticate) != 1 {
			t.Fatalf("%s did not fail closed: %d %s", testCase.name, response.Code, response.Body.String())
		}
		challenge := payload.Result.Meta.Authenticate[0]
		if !strings.Contains(challenge, `error="insufficient_scope"`) || !strings.Contains(challenge, `scope="`+compositionDiagnosticScope+" "+testCase.scope+`"`) {
			t.Fatalf("%s challenge did not request cumulative scopes: %s", testCase.name, challenge)
		}
	}
	if afterBytes, err := os.ReadFile(filepath.Join(root, "tracked.txt")); err != nil || string(afterBytes) != string(beforeBytes) {
		t.Fatalf("insufficient-scope calls changed bytes: %v %q", err, afterBytes)
	}
	if afterHead := gitOutput(t, root, "rev-parse", "HEAD"); afterHead != beforeHead {
		t.Fatalf("insufficient-scope calls changed HEAD: before=%s after=%s", beforeHead, afterHead)
	}
	if afterCached := gitOutput(t, root, "diff", "--cached", "--raw"); afterCached != beforeCached {
		t.Fatalf("insufficient-scope calls changed index: before=%s after=%s", beforeCached, afterCached)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(output))
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
