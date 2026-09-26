package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func TestPublicGitIndexPromotionIsManagedWorktreeOnly(t *testing.T) {
	handler, authorization, console, err := embeddedHandlerWithWorkspace(readTestResource, filepath.Join(t.TempDir(), "identity"), compositionProgramming)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = console.Close()
		_ = authorization.Close()
	})

	registered := readRequest(t, handler, http.MethodPost, "/register", fmt.Sprintf(`{"client_name":"Git index promotion test","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback), "application/json", "", nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", registered.Code, registered.Body.String())
	}
	var client struct {
		ID string `json:"client_id"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &client); err != nil || client.ID == "" {
		t.Fatalf("registration: %v", err)
	}
	authorizeClient(t, handler, func(id string) error { return authorization.DecideTerminal(id, true) }, client.ID, compositionDiagnosticScope)

	root := gitWorkspaceFixture(t)
	var output strings.Builder
	console.handleWorkspaceCommand("workspace request-programming "+client.ID+" "+workspace.ScopeGitIndex+" "+root, &output)
	if !strings.Contains(output.String(), "programming request rejected") || strings.Contains(output.String(), "solicitada") {
		t.Fatal("generic programming request unexpectedly accepted managed-only Git index")
	}

	managedScopes := strings.Join([]string{workspace.ScopeRead, workspace.ScopeWrite, workspace.ScopeGit, workspace.ScopeGitIndex}, ",")
	output.Reset()
	console.handleWorkspaceCommand("workspace request-worktree "+client.ID+" "+managedScopes+" "+root, &output)
	if console.managedPending == nil {
		t.Fatalf("managed worktree request was not created: %s", output.String())
	}
	console.handleWorkspaceCommand("workspace approve-worktree "+console.managedPending.id, &output)
	snapshot, err := console.grants.Snapshot()
	if err != nil || !snapshot.Active || snapshot.Mode != workspace.WorkspaceModeWorktree || !containsScope(snapshot.Scopes, workspace.ScopeGitIndex) {
		t.Fatalf("managed Git index grant was not created: %#v %v\n%s", snapshot, err, output.String())
	}
	indexToken := authorizeClient(t, handler, func(id string) error { return authorization.DecideTerminal(id, true) }, client.ID, compositionDiagnosticScope+" "+workspace.ScopeGitIndex)
	reviewToken := authorizeClient(t, handler, func(id string) error { return authorization.DecideTerminal(id, true) }, client.ID, compositionDiagnosticScope+" "+workspace.ScopeGit)
	allToken := authorizeClient(t, handler, func(id string) error { return authorization.DecideTerminal(id, true) }, client.ID, compositionDiagnosticScope+" "+workspace.ScopeRead+" "+workspace.ScopeWrite+" "+workspace.ScopeGit+" "+workspace.ScopeGitIndex)

	programmingTools := []string{"connection_diagnostic", "read_file", "list_directory", "stat_path", "find_paths", "search_text", "replace_text", "create_directory", "create_text_file", "write_text_file", "copy_path", "move_path", "delete_file", "delete_directory", "apply_patch", "review_git_changes", "git_status", "stage_git_paths", "unstage_git_paths", "commit_git_index"}
	assertPublicToolNames(t, handler, indexToken, programmingTools...)
	assertPublicToolNames(t, handler, reviewToken, programmingTools...)
	assertPublicToolNames(t, handler, allToken, programmingTools...)

	statusText, statusError := publicGitIndexToolText(t, handler, reviewToken, "git_status", map[string]string{"session_id": snapshot.SessionID})
	if statusError {
		t.Fatalf("git_status denied for review token: %s", statusText)
	}
	var before workspace.GitIndexStatus
	if err := json.Unmarshal([]byte(statusText), &before); err != nil || before.IndexSHA256 == "" {
		t.Fatalf("invalid public git status: %v %s", err, statusText)
	}

	// Mutate the managed fixture only through the local grant boundary. The MCP
	// response never carries an absolute path.
	var managedFile string
	if err := console.grants.WithAuthorizedManagedGitProcessDir(console.owner, client.ID, snapshot.SessionID, func(directory workspace.ProcessDirectory) error {
		return directory.WithProcessDir(func(root string) error {
			managedFile = filepath.Join(root, "tracked.txt")
			return os.WriteFile(managedFile, []byte("staged\n"), 0600)
		})
	}); err != nil {
		t.Fatal(err)
	}

	fileHash := sha256.Sum256([]byte("staged\n"))
	stageArgs := map[string]any{
		"session_id":            snapshot.SessionID,
		"expected_index_sha256": before.IndexSHA256,
		"entries":               []map[string]any{{"path": "tracked.txt", "expected_sha256": hex.EncodeToString(fileHash[:])}},
	}
	stageText, stageError := publicGitIndexToolText(t, handler, indexToken, "stage_git_paths", stageArgs)
	if stageError || !strings.Contains(stageText, `"status":"staged"`) {
		t.Fatalf("managed stage failed: error=%t text=%s", stageError, stageText)
	}

	stagedText, stagedError := publicGitIndexToolText(t, handler, reviewToken, "git_status", map[string]string{"session_id": snapshot.SessionID})
	if stagedError {
		t.Fatalf("git_status after stage failed: %s", stagedText)
	}
	var staged workspace.GitIndexStatus
	if err := json.Unmarshal([]byte(stagedText), &staged); err != nil {
		t.Fatal(err)
	}
	var entry workspace.GitIndexStatusEntry
	for _, candidate := range staged.Entries {
		if candidate.Path == "tracked.txt" {
			entry = candidate
		}
	}
	if !entry.Staged || entry.IndexObjectID == "" {
		t.Fatalf("public status did not expose staged entry: %+v", staged)
	}
	unstageArgs := map[string]any{
		"session_id":            snapshot.SessionID,
		"expected_index_sha256": staged.IndexSHA256,
		"entries":               []map[string]any{{"path": "tracked.txt", "expected_index_oid": entry.IndexObjectID}},
	}
	unstageText, unstageError := publicGitIndexToolText(t, handler, indexToken, "unstage_git_paths", unstageArgs)
	if unstageError || !strings.Contains(unstageText, `"status":"unstaged"`) {
		t.Fatalf("managed unstage failed: error=%t text=%s", unstageError, unstageText)
	}
	if managedFile == "" {
		t.Fatal("managed file path was not captured")
	}
	data, err := os.ReadFile(managedFile)
	if err != nil || string(data) != "staged\n" {
		t.Fatalf("unstage changed working-tree bytes: %v %q", err, data)
	}
}

func publicGitIndexToolText(t *testing.T, handler http.Handler, bearer, name string, arguments any) (string, bool) {
	t.Helper()
	params, err := json.Marshal(map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": json.RawMessage(params)})
	if err != nil {
		t.Fatal(err)
	}
	response := readRequest(t, handler, http.MethodPost, "/mcp", string(request), "application/json", bearer, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("%s HTTP status: %d %s", name, response.Code, response.Body.String())
	}
	var payload struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || len(payload.Result.Content) != 1 {
		t.Fatalf("%s response: %v %s", name, err, response.Body.String())
	}
	return payload.Result.Content[0].Text, payload.Result.IsError
}
