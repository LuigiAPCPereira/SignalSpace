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
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func writeCall(sessionID, relative, expected, replacement string) string {
	params, _ := json.Marshal(map[string]any{
		"name": writeToolName,
		"arguments": map[string]string{
			"session_id":  sessionID,
			"path":        relative,
			"expected":    expected,
			"replacement": replacement,
		},
	})
	request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": json.RawMessage(params)})
	return string(request)
}

func writeOAuthServer(t *testing.T, verifier *JWKSVerifier, grants *workspace.Grants) *httptest.Server {
	t.Helper()
	handler, err := NewOAuthHandler(OAuthConfig{
		ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: "owner-test",
		workspaceWriter: grants,
	}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func signedWriteToken(t *testing.T, key *rsa.PrivateKey, clientID, scope string) string {
	t.Helper()
	claims := defaultClaims()
	claims["client_id"] = clientID
	claims["scope"] = scope
	return makeAccessToken(t, key, claims)
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func toolNames(t *testing.T, result map[string]any) []string {
	t.Helper()
	tools := result["result"].(map[string]any)["tools"].([]any)
	names := make([]string, 0, len(tools))
	for _, raw := range tools {
		names = append(names, raw.(map[string]any)["name"].(string))
	}
	return names
}

func TestWorkspaceWriteToolIsolatedAuthorizationBoundary(t *testing.T) {
	key, verifier, _, diagnosticOnly := setupOAuth(t)
	grants, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grants.Close() })
	server := writeOAuthServer(t, verifier, grants)
	clientID := strings.Repeat("W", 32)
	otherClient := strings.Repeat("X", 32)
	root := t.TempDir()
	target := filepath.Join(root, "editable.txt")
	if err := os.WriteFile(target, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}

	res, metadata := oauthRequest(t, server, http.MethodGet, metadataPath, "", "", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("write test metadata status=%d", res.StatusCode)
	}
	if got := metadata["scopes_supported"].([]any); len(got) != 2 || got[1] != workspaceWriteScope {
		t.Fatalf("unexpected isolated scopes: %v", got)
	}

	writeToken := signedWriteToken(t, key, clientID, diagnosticScope+" "+workspaceWriteScope)
	res, result := oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("isolated tools/list status=%d", res.StatusCode)
	}
	if names := toolNames(t, result); len(names) != 2 || names[1] != writeToolName {
		t.Fatalf("isolated write tool missing: %v", names)
	}
	diagnosticOnlyToken := signedWriteToken(t, key, clientID, diagnosticScope)
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", diagnosticOnlyToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("diagnostic-only tools/list status=%d", res.StatusCode)
	}
	if names := toolNames(t, result); len(names) != 1 || names[0] == writeToolName {
		t.Fatalf("write tool advertised without write scope: %v", names)
	}

	sessionID, err := grants.GrantWithScopes(root, clientID, workspace.ScopeRead, workspace.ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	request := writeCall(sessionID, "editable.txt", "before", "after")
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, request, nil)
	text, isError := readResult(t, result)
	if res.StatusCode != http.StatusOK || isError || text != "Workspace text replaced." || fileText(t, target) != "after" {
		t.Fatalf("authorized write failed: %d %v file=%q", res.StatusCode, result, fileText(t, target))
	}

	if err := os.WriteFile(target, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := grants.Grant(root, clientID); err != nil {
		t.Fatal(err)
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, writeCall(sessionID, "editable.txt", "before", "denied"), nil)
	text, isError = readResult(t, result)
	if res.StatusCode != http.StatusOK || !isError || text == "Workspace text replaced." || fileText(t, target) != "before" {
		t.Fatalf("read-only grant authorized write: %d %v file=%q", res.StatusCode, result, fileText(t, target))
	}

	writeSession, err := grants.GrantWithScopes(root, clientID, workspace.ScopeRead, workspace.ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	readToken := signedReadToken(t, key, clientID)
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", readToken, writeCall(writeSession, "editable.txt", "before", "denied"), nil)
	text, isError = readResult(t, result)
	if res.StatusCode != http.StatusOK || !isError || text == "Workspace text replaced." || fileText(t, target) != "before" {
		t.Fatalf("read-only token authorized write: %d %v", res.StatusCode, result)
	}
	meta := result["result"].(map[string]any)["_meta"].(map[string]any)
	if !strings.Contains(meta["mcp/www_authenticate"].([]any)[0].(string), workspaceWriteScope) {
		t.Fatalf("missing write challenge: %v", meta)
	}
	diagnosticToken := signedWriteToken(t, key, clientID, diagnosticScope)
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", diagnosticToken, writeCall(writeSession, "editable.txt", "before", "denied"), nil)
	text, isError = readResult(t, result)
	if res.StatusCode != http.StatusOK || !isError || text == "Workspace text replaced." || fileText(t, target) != "before" {
		t.Fatalf("token without write scope authorized write: %d %v", res.StatusCode, result)
	}

	clientSession, err := grants.GrantWithScopes(root, clientID, workspace.ScopeRead, workspace.ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", signedWriteToken(t, key, otherClient, diagnosticScope+" "+workspaceWriteScope), writeCall(clientSession, "editable.txt", "before", "denied"), nil)
	text, isError = readResult(t, result)
	if res.StatusCode != http.StatusOK || !isError || text == "Workspace text replaced." || fileText(t, target) != "before" {
		t.Fatalf("different client reused write grant: %d %v", res.StatusCode, result)
	}

	ownerSession, err := grants.GrantWithScopes(root, clientID, workspace.ScopeRead, workspace.ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	ownerTokenClaims := defaultClaims()
	ownerTokenClaims["client_id"] = clientID
	ownerTokenClaims["scope"] = diagnosticScope + " " + workspaceWriteScope
	ownerTokenClaims["sub"] = "other-owner"
	res, _ = oauthRequest(t, server, http.MethodPost, "/mcp", makeAccessToken(t, key, ownerTokenClaims), writeCall(ownerSession, "editable.txt", "before", "denied"), nil)
	if res.StatusCode != http.StatusUnauthorized || fileText(t, target) != "before" {
		t.Fatalf("different owner reached write boundary: %d", res.StatusCode)
	}

	currentSession, err := grants.GrantWithScopes(root, clientID, workspace.ScopeRead, workspace.ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, writeCall(strings.Repeat("0", 32), "editable.txt", "before", "denied"), nil)
	text, isError = readResult(t, result)
	if res.StatusCode != http.StatusOK || !isError || fileText(t, target) != "before" {
		t.Fatalf("wrong session authorized write: %d %v", res.StatusCode, result)
	}

	otherGrants, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = otherGrants.Close() })
	otherRoot := t.TempDir()
	otherSession, err := otherGrants.GrantWithScopes(otherRoot, clientID, workspace.ScopeRead, workspace.ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, writeCall(otherSession, "editable.txt", "before", "denied"), nil)
	text, isError = readResult(t, result)
	if res.StatusCode != http.StatusOK || !isError || fileText(t, target) != "before" {
		t.Fatalf("different workspace grant authorized write: %d %v", res.StatusCode, result)
	}

	if err := grants.Revoke(currentSession); err != nil {
		t.Fatal(err)
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, writeCall(currentSession, "editable.txt", "before", "denied"), nil)
	text, isError = readResult(t, result)
	if res.StatusCode != http.StatusOK || !isError || fileText(t, target) != "before" {
		t.Fatalf("revoked grant authorized write: %d %v", res.StatusCode, result)
	}

	for _, tc := range []struct {
		name  string
		token string
		want  int
	}{
		{name: "missing", token: "", want: http.StatusUnauthorized},
		{name: "tampered", token: writeToken + "invalid", want: http.StatusUnauthorized},
		{name: "expired", token: func() string {
			claims := defaultClaims()
			claims["client_id"] = clientID
			claims["scope"] = diagnosticScope + " " + workspaceWriteScope
			claims["exp"] = time.Now().Add(-time.Minute).Unix()
			return makeAccessToken(t, key, claims)
		}(), want: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, _ := oauthRequest(t, server, http.MethodPost, "/mcp", tc.token, writeCall(currentSession, "editable.txt", "before", "denied"), nil)
			if res.StatusCode != tc.want || fileText(t, target) != "before" {
				t.Fatalf("invalid token reached write: %d want %d", res.StatusCode, tc.want)
			}
		})
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"replace_text","arguments":{"session_id":"`+writeSession+`","path":"editable.txt","expected":null,"replacement":"denied"}}}`, nil)
	if res.StatusCode != http.StatusOK || result["error"] == nil || fileText(t, target) != "before" {
		t.Fatalf("null expected value accepted: %d %v", res.StatusCode, result)
	}

	writeSession, err = grants.GrantWithScopes(root, clientID, workspace.ScopeRead, workspace.ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, expected, replacement string
	}{
		{name: "traversal", path: "../outside.txt", expected: "before", replacement: "escaped"},
		{name: "conflict", path: "editable.txt", expected: "wrong-version", replacement: "conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, result := oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, writeCall(writeSession, tc.path, tc.expected, tc.replacement), nil)
			text, isError := readResult(t, result)
			if res.StatusCode != http.StatusOK || !isError || text == "Workspace text replaced." || fileText(t, target) != "before" || fileText(t, outside) != "outside" {
				t.Fatalf("unsafe write changed state: %d %v", res.StatusCode, result)
			}
		})
	}

	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, writeCall(writeSession, "editable.txt", "before", "after"), nil)
	if text, isError := readResult(t, result); res.StatusCode != http.StatusOK || isError || text != "Workspace text replaced." || fileText(t, target) != "after" {
		t.Fatalf("write setup for duplicate test failed: %d %v", res.StatusCode, result)
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, writeCall(writeSession, "editable.txt", "before", "after"), nil)
	text, isError = readResult(t, result)
	if res.StatusCode != http.StatusOK || !isError || text == "Workspace text replaced." || fileText(t, target) != "after" {
		t.Fatalf("duplicate write repeated effect: %d %v", res.StatusCode, result)
	}

	validDiagnostic := makeAccessToken(t, key, defaultClaims())
	res, result = oauthRequest(t, diagnosticOnly, http.MethodPost, "/mcp", validDiagnostic, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("diagnostic tools/list status=%d", res.StatusCode)
	}
	if names := toolNames(t, result); len(names) != 1 || names[0] == writeToolName {
		t.Fatalf("public diagnostic tools changed: %v", names)
	}
	res, result = oauthRequest(t, diagnosticOnly, http.MethodPost, "/mcp", validDiagnostic, writeCall("session", "editable.txt", "before", "unexpected"), nil)
	if res.StatusCode != http.StatusOK || result["error"] == nil || fileText(t, target) != "after" {
		t.Fatalf("unregistered public write became available: %d %v", res.StatusCode, result)
	}
}
