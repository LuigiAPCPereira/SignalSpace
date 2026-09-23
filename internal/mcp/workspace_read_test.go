package mcp

import (
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func readCall(sessionID, relative string) string {
	params, _ := json.Marshal(map[string]any{"name": readToolName, "arguments": map[string]string{"session_id": sessionID, "path": relative}})
	request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": json.RawMessage(params)})
	return string(request)
}

func readResult(t *testing.T, result map[string]any) (string, bool) {
	t.Helper()
	if result["error"] != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", result)
	}
	payload := result["result"].(map[string]any)
	content := payload["content"].([]any)[0].(map[string]any)["text"].(string)
	return content, payload["isError"].(bool)
}

func readOAuthServer(t *testing.T, verifier *JWKSVerifier, grants *workspace.Grants) *httptest.Server {
	t.Helper()
	handler, err := NewOAuthHandler(OAuthConfig{
		ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: "owner-test", WorkspaceReader: grants,
	}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func signedReadToken(t *testing.T, key *rsa.PrivateKey, clientID string) string {
	t.Helper()
	claims := defaultClaims()
	claims["client_id"] = clientID
	claims["scope"] = diagnosticScope + " " + workspaceReadScope
	return makeAccessToken(t, key, claims)
}

func TestWorkspaceToolRequiresExplicitConfigurationAndIdentityVerifier(t *testing.T) {
	key, _, _, diagnosticOnly := setupOAuth(t)
	client := strings.Repeat("A", 32)
	token := signedReadToken(t, key, client)
	res, result := oauthRequest(t, diagnosticOnly, http.MethodPost, "/mcp", token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if res.StatusCode != 200 || len(result["result"].(map[string]any)["tools"].([]any)) != 1 {
		t.Fatal("workspace tool leaked into diagnostic mode")
	}
	res, result = oauthRequest(t, diagnosticOnly, http.MethodPost, "/mcp", token, readCall("session", "readme.txt"), nil)
	if res.StatusCode != 200 || result["error"] == nil {
		t.Fatal("diagnostic mode accepted read_file")
	}
	grant, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer grant.Close()
	if _, err := NewOAuthHandler(OAuthConfig{
		ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: "owner-test", WorkspaceReader: grant,
	}, fakeVerifier{}); err == nil {
		t.Fatal("read endpoint accepted verifier without trusted client identity")
	}
}

func TestWorkspaceReadToolAuthGrantRevocationAndPaths(t *testing.T) {
	key, verifier, _, _ := setupOAuth(t)
	grants, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grants.Close() })
	server := readOAuthServer(t, verifier, grants)
	clientID := strings.Repeat("B", 32)
	otherClient := strings.Repeat("C", 32)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("hello from approved workspace"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(strings.Repeat("X", workspace.MaxTextBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "binary.txt"), []byte{0, 1}, 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("OUTSIDE_SENSITIVE_MARKER"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}

	res, metadata := oauthRequest(t, server, http.MethodGet, metadataPath, "", "", nil)
	if res.StatusCode != 200 || len(metadata["scopes_supported"].([]any)) != 2 {
		t.Fatal("read scope metadata missing from opt-in resource")
	}
	readToken := signedReadToken(t, key, clientID)
	res, result := oauthRequest(t, server, http.MethodPost, "/mcp", readToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if res.StatusCode != 200 {
		t.Fatalf("tool discovery failed: %d", res.StatusCode)
	}
	tools := result["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 || tools[1].(map[string]any)["name"] != readToolName {
		t.Fatalf("unexpected opt-in tools: %v", tools)
	}
	schemes := tools[1].(map[string]any)["securitySchemes"].([]any)
	if schemes[0].(map[string]any)["scopes"].([]any)[0] != workspaceReadScope {
		t.Fatalf("read tool missing separate OAuth scope: %v", schemes)
	}
	// A configuração local de leitura não deve anunciar a ferramenta a um
	// bearer que só possui o escopo diagnóstico.
	diagnosticClaims := defaultClaims()
	diagnosticClaims["client_id"] = clientID
	diagnosticToken := makeAccessToken(t, key, diagnosticClaims)
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", diagnosticToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if res.StatusCode != 200 {
		t.Fatalf("diagnostic tool discovery failed: %d", res.StatusCode)
	}
	tools = result["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != toolName {
		t.Fatalf("diagnostic token advertised workspace tools: %v", tools)
	}

	// Uma autorização OAuth não concede uma raiz nem um ID de sessão.
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", readToken, readCall(strings.Repeat("0", 32), "readme.txt"), nil)
	if text, isError := readResult(t, result); res.StatusCode != 200 || !isError || strings.Contains(text, "hello") {
		t.Fatal("read succeeded without local grant")
	}
	sessionID, err := grants.Grant(root, clientID)
	if err != nil {
		t.Fatal(err)
	}
	request := readCall(sessionID, "readme.txt")

	// O bearer de diagnóstico não permite a operação, mesmo com a sessão correta.
	diagnostic := defaultClaims()
	diagnostic["client_id"] = clientID
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", makeAccessToken(t, key, diagnostic), request, nil)
	text, isError := readResult(t, result)
	if res.StatusCode != 200 || !isError || strings.Contains(text, "hello") || !strings.Contains(result["result"].(map[string]any)["_meta"].(map[string]any)["mcp/www_authenticate"].([]any)[0].(string), workspaceReadScope) {
		t.Fatalf("diagnostic token could read files: %d %v", res.StatusCode, result)
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", signedReadToken(t, key, otherClient), request, nil)
	if text, isError := readResult(t, result); res.StatusCode != 200 || !isError || strings.Contains(text, "hello") {
		t.Fatal("different OAuth client reused workspace grant")
	}

	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", readToken, request, nil)
	if text, isError := readResult(t, result); res.StatusCode != 200 || isError || text != "hello from approved workspace" {
		t.Fatalf("legitimate read rejected: %d %v", res.StatusCode, result)
	}
	for _, tc := range []struct{ name, payload string }{
		{"invalid_session", readCall(strings.Repeat("a", 32), "readme.txt")},
		{"traversal", readCall(sessionID, "../outside.txt")},
		{"absolute_path", readCall(sessionID, outside)},
		{"symlink_escape", readCall(sessionID, "escape.txt")},
		{"oversized", readCall(sessionID, "large.txt")},
		{"binary", readCall(sessionID, "binary.txt")},
		{"missing", readCall(sessionID, "unknown.txt")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, result := oauthRequest(t, server, http.MethodPost, "/mcp", readToken, tc.payload, nil)
			text, isError := readResult(t, result)
			if res.StatusCode != 200 || !isError || strings.Contains(text, "OUTSIDE_SENSITIVE_MARKER") || strings.Contains(text, root) {
				t.Fatalf("unsafe read: %d %v", res.StatusCode, result)
			}
		})
	}
	for _, invalid := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"readme.txt"}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"session_id":"id","path":"readme.txt","root":"/"}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"session_id":1,"path":"readme.txt"}}}`,
	} {
		res, result := oauthRequest(t, server, http.MethodPost, "/mcp", readToken, invalid, nil)
		if res.StatusCode != 200 || result["error"] == nil {
			t.Fatalf("accepted malformed arguments: %v", result)
		}
	}
	// Substituir uma concessão também invalida o ID antigo.
	replacement, err := grants.Grant(root, clientID)
	if err != nil || replacement == sessionID {
		t.Fatalf("invalid replacement grant: %q %v", replacement, err)
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", readToken, request, nil)
	if text, isError := readResult(t, result); res.StatusCode != 200 || !isError || strings.Contains(text, "hello") {
		t.Fatal("superseded session continued reading")
	}
	request = readCall(replacement, "readme.txt")
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", readToken, request, nil)
	if text, isError := readResult(t, result); res.StatusCode != 200 || isError || text != "hello from approved workspace" {
		t.Fatal("replacement session denied")
	}
	// A concessão precisa ser consultada por chamada, mesmo antes de expirar JWT.
	if err := grants.Revoke(replacement); err != nil {
		t.Fatal(err)
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", readToken, request, nil)
	if text, isError := readResult(t, result); res.StatusCode != 200 || !isError || strings.Contains(text, "hello") {
		t.Fatal("read after revocation succeeded")
	}
}

func TestWorkspaceReadRejectsInvalidTokens(t *testing.T) {
	key, verifier, _, _ := setupOAuth(t)
	grants, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	server := readOAuthServer(t, verifier, grants)
	client := strings.Repeat("D", 32)
	token := signedReadToken(t, key, client)
	body := readCall(strings.Repeat("0", 32), "readme.txt")
	for _, tc := range []struct{ name, token string }{
		{"missing", ""},
		{"tampered", token + "invalid"},
		{"legacy", makeAccessToken(t, key, func() map[string]any {
			c := defaultClaims()
			c["scope"] = diagnosticScope + " " + workspaceReadScope
			return c
		}())},
		{"wrong_owner", makeAccessToken(t, key, func() map[string]any {
			c := defaultClaims()
			c["scope"] = diagnosticScope + " " + workspaceReadScope
			c["sub"] = "other"
			c["client_id"] = client
			return c
		}())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, result := oauthRequest(t, server, http.MethodPost, "/mcp", tc.token, body, nil)
			if tc.name == "legacy" {
				_, isError := readResult(t, result)
				if res.StatusCode != 200 || !isError {
					t.Fatalf("legacy token unexpectedly authorized: %d", res.StatusCode)
				}
			} else if res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("invalid bearer accepted: %d %v", res.StatusCode, result)
			}
		})
	}
	// Verificador continua podendo servir diagnóstico sem autorização de workspace.
	res, result := oauthRequest(t, server, http.MethodPost, "/mcp", token, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"connection_diagnostic","arguments":{}}}`, nil)
	if res.StatusCode != 200 || result["error"] != nil {
		t.Fatalf("diagnostic regression: %d %v", res.StatusCode, result)
	}
}
