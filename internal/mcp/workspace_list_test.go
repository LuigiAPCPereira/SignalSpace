package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func listCall(sessionID, relative string) string {
	params, _ := json.Marshal(map[string]any{"name": listDirectoryToolName, "arguments": map[string]string{"session_id": sessionID, "path": relative}})
	request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": json.RawMessage(params)})
	return string(request)
}

func listOAuthServer(t *testing.T, verifier *JWKSVerifier, grants *workspace.Grants) *httptest.Server {
	t.Helper()
	handler, err := NewOAuthHandler(OAuthConfig{
		ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: "owner-test", WorkspaceReader: grants, WorkspaceLister: grants,
	}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func assertListDenied(t *testing.T, server *httptest.Server, token, body string) {
	t.Helper()
	res, result := oauthRequest(t, server, http.MethodPost, "/mcp", token, body, nil)
	text, isError := readResult(t, result)
	if res.StatusCode != http.StatusOK || !isError || text != "Workspace directory listing unavailable or not authorized." {
		t.Fatalf("directory listing leaked information: status=%d result=%v", res.StatusCode, result)
	}
}

func listEntries(t *testing.T, server *httptest.Server, token, body string, want []string) {
	t.Helper()
	res, result := oauthRequest(t, server, http.MethodPost, "/mcp", token, body, nil)
	text, isError := readResult(t, result)
	if res.StatusCode != http.StatusOK || isError {
		t.Fatalf("authorized listing denied: status=%d result=%v", res.StatusCode, result)
	}
	var parsed struct {
		Entries []string `json:"entries"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil || !reflect.DeepEqual(parsed.Entries, want) {
		t.Fatalf("unexpected bounded listing: %q, %v (expected %v)", text, err, want)
	}
	if want != nil && strings.Contains(text, "PRIVATE_FILE_CONTENT") {
		t.Fatal("directory listing returned file content")
	}
}

func TestDirectoryToolRequiresExplicitPortsAndVerifiedIdentity(t *testing.T) {
	key, verifier, _, diagnosticOnly := setupOAuth(t)
	clientID := strings.Repeat("A", 32)
	token := signedReadToken(t, key, clientID)
	grants, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	readOnly := readOAuthServer(t, verifier, grants)
	for _, tc := range []struct {
		name   string
		server *httptest.Server
		count  int
	}{
		{"diagnostic", diagnosticOnly, 1},
		{"read_without_lister", readOnly, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, result := oauthRequest(t, tc.server, http.MethodPost, "/mcp", token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
			if res.StatusCode != http.StatusOK || len(result["result"].(map[string]any)["tools"].([]any)) != tc.count {
				t.Fatalf("unexpected tools: %d %v", res.StatusCode, result)
			}
			res, result = oauthRequest(t, tc.server, http.MethodPost, "/mcp", token, listCall("unknown", "."), nil)
			if res.StatusCode != http.StatusOK || result["error"] == nil {
				t.Fatalf("unconfigured listing accepted: %v", result)
			}
		})
	}
	if _, err := NewOAuthHandler(OAuthConfig{
		ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: "owner-test", WorkspaceLister: grants,
	}, verifier); err == nil {
		t.Fatal("directory listing configured without file reader")
	}
	if _, err := NewOAuthHandler(OAuthConfig{
		ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: "owner-test", WorkspaceReader: grants, WorkspaceLister: grants,
	}, fakeVerifier{}); err == nil {
		t.Fatal("directory listing accepted verifier without signed client identity")
	}
}

func TestDirectoryToolRequiresReadScopeAndCurrentGrant(t *testing.T) {
	key, verifier, _, _ := setupOAuth(t)
	grants, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	server := listOAuthServer(t, verifier, grants)
	clientID := strings.Repeat("B", 32)
	otherClient := strings.Repeat("C", 32)
	readToken := signedReadToken(t, key, clientID)
	res, result := oauthRequest(t, server, http.MethodPost, "/mcp", readToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("tool discovery: %d %v", res.StatusCode, result)
	}
	tools := result["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 3 || tools[0].(map[string]any)["name"] != toolName || tools[1].(map[string]any)["name"] != readToolName || tools[2].(map[string]any)["name"] != listDirectoryToolName {
		t.Fatalf("wrong opt-in tools: %v", tools)
	}
	listing := tools[2].(map[string]any)
	if listing["securitySchemes"].([]any)[0].(map[string]any)["scopes"].([]any)[0] != workspaceReadScope {
		t.Fatal("directory listing was not bound to read scope")
	}
	schema := listing["inputSchema"].(map[string]any)
	if schema["additionalProperties"] != false || len(schema["required"].([]any)) != 2 {
		t.Fatal("directory listing has an unsafe argument schema")
	}
	request := listCall(strings.Repeat("0", 32), ".")
	assertListDenied(t, server, readToken, request)

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "z.txt"), []byte("PRIVATE_FILE_CONTENT"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "inside.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "OUTSIDE_MARKER"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	session, err := grants.Grant(root, clientID)
	if err != nil {
		t.Fatal(err)
	}
	request = listCall(session, ".")

	// Um bearer de diagnóstico não autoriza listagem, mesmo com o ID correto.
	diagnostic := defaultClaims()
	diagnostic["client_id"] = clientID
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", makeAccessToken(t, key, diagnostic), request, nil)
	text, isError := readResult(t, result)
	if res.StatusCode != http.StatusOK || !isError || strings.Contains(text, "nested") || !strings.Contains(result["result"].(map[string]any)["_meta"].(map[string]any)["mcp/www_authenticate"].([]any)[0].(string), workspaceReadScope) {
		t.Fatalf("diagnostic scope allowed listing: %v", result)
	}
	assertListDenied(t, server, signedReadToken(t, key, otherClient), request)
	for _, tc := range []struct{ name, token string }{
		{"missing", ""},
		{"tampered", readToken + "invalid"},
		{"other_owner", makeAccessToken(t, key, func() map[string]any {
			claims := defaultClaims()
			claims["client_id"] = clientID
			claims["scope"] = diagnosticScope + " " + workspaceReadScope
			claims["sub"] = "other-owner"
			return claims
		}())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, result := oauthRequest(t, server, http.MethodPost, "/mcp", tc.token, request, nil)
			if res.StatusCode != http.StatusUnauthorized || strings.Contains(strings.TrimSpace(resultString(result)), "nested") {
				t.Fatalf("invalid token accessed directory: %d %v", res.StatusCode, result)
			}
		})
	}
	listEntries(t, server, readToken, request, []string{"link", "nested", "z.txt"})
	listEntries(t, server, readToken, listCall(session, "nested"), []string{"inside.txt"})
	for _, path := range []string{"../outside", "/etc", "nested/..", "nested//", "link", "z.txt", "unknown"} {
		assertListDenied(t, server, readToken, listCall(session, path))
	}
	for _, raw := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_directory","arguments":{"path":"."}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_directory","arguments":{"session_id":"id","path":".","root":"/"}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_directory","arguments":{"session_id":1,"path":"."}}}`,
	} {
		res, result := oauthRequest(t, server, http.MethodPost, "/mcp", readToken, raw, nil)
		if res.StatusCode != http.StatusOK || result["error"] == nil {
			t.Fatalf("malformed listing accepted: %v", result)
		}
	}
	// O limite não retorna uma lista incompleta ao cliente remoto.
	if err := os.Mkdir(filepath.Join(root, "overflow"), 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= workspace.MaxDirectoryEntries; i++ {
		if err := os.WriteFile(filepath.Join(root, "overflow", string(rune(0x100+i))), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	assertListDenied(t, server, readToken, listCall(session, "overflow"))

	// A substituição não pode conservar o ID antigo, mesmo com JWT válido.
	replacement, err := grants.Grant(root, clientID)
	if err != nil || replacement == session {
		t.Fatalf("replacement failed: %v", err)
	}
	assertListDenied(t, server, readToken, request)
	request = listCall(replacement, "nested")
	listEntries(t, server, readToken, request, []string{"inside.txt"})
	if err := grants.Revoke(replacement); err != nil {
		t.Fatal(err)
	}
	assertListDenied(t, server, readToken, request)
	if err := grants.Close(); err != nil {
		t.Fatal(err)
	}
	assertListDenied(t, server, readToken, request)
}

// resultString evita assumir o formato de uma resposta HTTP não autorizada.
func resultString(result map[string]any) string {
	if result == nil {
		return ""
	}
	value, _ := json.Marshal(result)
	return string(value)
}
