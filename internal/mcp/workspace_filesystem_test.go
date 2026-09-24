package mcp

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func typedFilesystemOAuthServer(t *testing.T, verifier *JWKSVerifier, grants *workspace.Grants) *httptest.Server {
	t.Helper()
	handler, err := NewOAuthHandler(OAuthConfig{
		ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: "owner-test",
		WorkspaceReader: grants, WorkspaceLister: grants,
		workspaceStatter: grants, workspaceFinder: grants, workspaceSearcher: grants,
		workspaceWriter: grants, workspaceDirectoryCreator: grants,
		workspaceTextCreator: grants, workspaceTextUpdater: grants,
	}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func typedToken(t *testing.T, key *rsa.PrivateKey, clientID, scope string) string {
	t.Helper()
	claims := defaultClaims()
	claims["client_id"] = clientID
	claims["scope"] = scope
	return makeAccessToken(t, key, claims)
}

func callTool(name, arguments string) string {
	return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + arguments + `}}`
}

func TestTypedFilesystemMCPScopesSchemasAndStructuredResults(t *testing.T) {
	key, verifier, _, _ := setupOAuth(t)
	grants, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grants.Close() })
	server := typedFilesystemOAuthServer(t, verifier, grants)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\nneedle\n"), 0600); err != nil {
		t.Fatal(err)
	}
	clientID := strings.Repeat("F", 32)
	sessionID, err := grants.GrantWithScopes(root, clientID, workspace.ScopeRead, workspace.ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	readToken := typedToken(t, key, clientID, diagnosticScope+" "+workspace.ScopeRead)
	writeToken := typedToken(t, key, clientID, diagnosticScope+" "+workspace.ScopeWrite)

	_, readList := oauthRequest(t, server, http.MethodPost, "/mcp", readToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if names := toolNames(t, readList); len(names) != 6 || names[3] != statPathToolName || names[4] != findPathsToolName || names[5] != searchTextToolName {
		t.Fatalf("read tools were not typed and ordered: %v", names)
	}
	_, writeList := oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if names := toolNames(t, writeList); len(names) != 5 || names[1] != writeToolName || names[2] != createDirectoryToolName || names[3] != createTextFileToolName || names[4] != writeTextFileToolName {
		t.Fatalf("write tools were not typed and ordered: %v", names)
	}

	statResponse, statResult := oauthRequest(t, server, http.MethodPost, "/mcp", readToken, callTool(statPathToolName, `{"session_id":"`+sessionID+`","path":"src/main.go"}`), nil)
	if statResponse.StatusCode != http.StatusOK || !strings.Contains(statResult["result"].(map[string]any)["structuredContent"].(map[string]any)["kind"].(string), "regular_file") {
		t.Fatalf("structured stat result missing: %d %v", statResponse.StatusCode, statResult)
	}
	findResponse, findResult := oauthRequest(t, server, http.MethodPost, "/mcp", readToken, callTool(findPathsToolName, `{"session_id":"`+sessionID+`","pattern":"*.go"}`), nil)
	if findResponse.StatusCode != http.StatusOK || !strings.Contains(findResult["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string), "src/main.go") {
		t.Fatalf("structured find result missing: %d %v", findResponse.StatusCode, findResult)
	}
	searchResponse, searchResult := oauthRequest(t, server, http.MethodPost, "/mcp", readToken, callTool(searchTextToolName, `{"session_id":"`+sessionID+`","query":"needle"}`), nil)
	if searchResponse.StatusCode != http.StatusOK || !strings.Contains(searchResult["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string), "src/main.go") {
		t.Fatalf("structured search result missing: %d %v", searchResponse.StatusCode, searchResult)
	}

	createDirectoryResponse, createDirectoryResult := oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, callTool(createDirectoryToolName, `{"session_id":"`+sessionID+`","path":"generated"}`), nil)
	if createDirectoryResponse.StatusCode != http.StatusOK || !strings.Contains(createDirectoryResult["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string), `"status":"created"`) {
		t.Fatalf("structured directory result missing: %d %v", createDirectoryResponse.StatusCode, createDirectoryResult)
	}
	createFileResponse, createFileResult := oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, callTool(createTextFileToolName, `{"session_id":"`+sessionID+`","path":"generated/file.txt","content":"before"}`), nil)
	if createFileResponse.StatusCode != http.StatusOK || !strings.Contains(createFileResult["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string), `"status":"created"`) {
		t.Fatalf("structured file result missing: %d %v", createFileResponse.StatusCode, createFileResult)
	}
	var fileSum [32]byte
	fileSum = sha256.Sum256([]byte("before"))
	updateResponse, updateResult := oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, callTool(writeTextFileToolName, `{"session_id":"`+sessionID+`","path":"generated/file.txt","expected_sha256":"`+hex.EncodeToString(fileSum[:])+`","content":"after"}`), nil)
	if updateResponse.StatusCode != http.StatusOK || !strings.Contains(updateResult["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string), `"status":"updated"`) {
		t.Fatalf("structured update result missing: %d %v", updateResponse.StatusCode, updateResult)
	}

	readCreateResponse, readCreateResult := oauthRequest(t, server, http.MethodPost, "/mcp", readToken, callTool(createTextFileToolName, `{"session_id":"`+sessionID+`","path":"blocked.txt","content":"no"}`), nil)
	if readCreateResponse.StatusCode != http.StatusOK || !strings.Contains(readCreateResult["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string), "Workspace write authorization required") {
		t.Fatalf("read bearer reached write tool: %d %v", readCreateResponse.StatusCode, readCreateResult)
	}
	invalidResponse, invalidResult := oauthRequest(t, server, http.MethodPost, "/mcp", readToken, callTool(statPathToolName, `{"session_id":"`+sessionID+`","path":"src/main.go","extra":true}`), nil)
	if invalidResponse.StatusCode != http.StatusOK || invalidResult["error"] == nil {
		t.Fatalf("stat accepted extra arguments: %d %v", invalidResponse.StatusCode, invalidResult)
	}
}
