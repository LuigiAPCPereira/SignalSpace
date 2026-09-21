package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const oauthWriteCallback = "https://chatgpt.com/callback"

type oauthHarnessResponse struct {
	status int
	header http.Header
	body   []byte
}

func oauthWriteHTTP(t *testing.T, server *httptest.Server, method, path, contentType, body, token, cookie string) oauthHarnessResponse {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "signalspace.example"
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if path == "/mcp" {
		req.Header.Set("MCP-Protocol-Version", protocolVersion)
	}
	client := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return oauthHarnessResponse{status: response.StatusCode, header: response.Header.Clone(), body: data}
}

func oauthWriteJSON(t *testing.T, response oauthHarnessResponse) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(response.body, &data); err != nil {
		t.Fatalf("invalid JSON status=%d body=%s: %v", response.status, response.body, err)
	}
	return data
}

func oauthWriteCall(sessionID, path, expected, replacement string) string {
	arguments, _ := json.Marshal(map[string]string{
		"session_id":  sessionID,
		"path":        path,
		"expected":    expected,
		"replacement": replacement,
	})
	request, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      writeToolName,
			"arguments": json.RawMessage(arguments),
		},
	})
	return string(request)
}

func oauthWriteClaims(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("issued access token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

func TestOAuthWriteVerticalFlowUsesRealIssuerVerifierAndRevocation(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "editable.txt")
	if err := os.WriteFile(file, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}

	composition := newOAuthWriteComposition(t, filepath.Join(t.TempDir(), "identity"))
	clientID, sessionID, accessToken, tokenForm := issueOAuthWriteToken(t, composition, root)
	server := composition.server
	grants := composition.grants
	authorization := composition.authorization
	verifier := composition.verifier
	identity, err := verifier.VerifyIdentity(context.Background(), accessToken, oauthWriteIssuer, testResource, workspaceWriteScope, authorization.OwnerSubject())
	if err != nil || identity.ClientID != clientID {
		t.Fatalf("real verifier rejected issued write identity: %+v %v", identity, err)
	}
	if err := verifier.Verify(context.Background(), accessToken, oauthWriteIssuer, testResource, diagnosticScope, authorization.OwnerSubject()); err != nil {
		t.Fatalf("issued token lost diagnostic scope: %v", err)
	}

	listResponse := oauthWriteHTTP(t, server, http.MethodPost, "/mcp", "application/json", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, accessToken, "")
	if listResponse.status != http.StatusOK {
		t.Fatalf("MCP tools/list failed: %d %s", listResponse.status, listResponse.body)
	}
	list := oauthWriteJSON(t, listResponse)
	tools := list["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 || tools[1].(map[string]any)["name"] != writeToolName {
		t.Fatalf("real OAuth write token did not discover replace_text: %v", tools)
	}

	editResponse := oauthWriteHTTP(t, server, http.MethodPost, "/mcp", "application/json", oauthWriteCall(sessionID, "editable.txt", "before", "after"), accessToken, "")
	edit := oauthWriteJSON(t, editResponse)
	if editResponse.status != http.StatusOK || edit["result"].(map[string]any)["isError"] != false {
		t.Fatalf("real OAuth write edit failed: %d %s", editResponse.status, editResponse.body)
	}
	content, err := os.ReadFile(file)
	if err != nil || string(content) != "after" {
		t.Fatalf("MCP edit did not change the disposable workspace: %q %v", content, err)
	}

	if err := grants.Revoke(sessionID); err != nil {
		t.Fatal(err)
	}
	revokedResponse := oauthWriteHTTP(t, server, http.MethodPost, "/mcp", "application/json", oauthWriteCall(sessionID, "editable.txt", "after", "after-again"), accessToken, "")
	revoked := oauthWriteJSON(t, revokedResponse)
	if revokedResponse.status != http.StatusOK || revoked["result"].(map[string]any)["isError"] != true {
		t.Fatalf("revoked grant retained write access: %d %s", revokedResponse.status, revokedResponse.body)
	}
	content, err = os.ReadFile(file)
	if err != nil || string(content) != "after" {
		t.Fatalf("revoked write changed the workspace: %q %v", content, err)
	}

	if replay := oauthWriteHTTP(t, server, http.MethodPost, "/token", "application/x-www-form-urlencoded", tokenForm.Encode(), "", ""); replay.status != http.StatusBadRequest {
		t.Fatalf("authorization code was reusable: %d %s", replay.status, replay.body)
	}

	publicHandler, err := NewOAuthHandler(OAuthConfig{
		ResourceURL:  testResource,
		Issuer:       "https://signalspace.example",
		OwnerSubject: authorization.OwnerSubject(),
	}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	publicServer := httptest.NewServer(publicHandler)
	t.Cleanup(publicServer.Close)
	publicListResponse, publicList := oauthRequest(t, publicServer, http.MethodPost, "/mcp", accessToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if publicListResponse.StatusCode != http.StatusOK || len(publicList["result"].(map[string]any)["tools"].([]any)) != 1 {
		t.Fatalf("public MCP composition exposed write tool: %d %v", publicListResponse.StatusCode, publicList)
	}
	publicCallResponse, publicCall := oauthRequest(t, publicServer, http.MethodPost, "/mcp", accessToken, oauthWriteCall(sessionID, "editable.txt", "after", "public"), nil)
	if publicCallResponse.StatusCode != http.StatusOK || publicCall["error"] == nil {
		t.Fatalf("public MCP composition accepted unregistered write: %d %v", publicCallResponse.StatusCode, publicCall)
	}
}
