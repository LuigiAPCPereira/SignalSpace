package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
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

	requests := make(chan auth.RequestInfo, 4)
	var grants *workspace.Grants
	authorization, err := auth.New(auth.Config{
		ResourceURL: testResource,
		Issuer:      "https://signalspace.example",
		Scope:       diagnosticScope,
		WriteScope:  workspaceWriteScope,
		CanIssueWrite: func(clientID string) bool {
			return grants != nil && grants.AllowsClientScope(clientID, workspace.ScopeWrite)
		},
		StateDir: filepath.Join(t.TempDir(), "identity"),
		OnRequest: func(info auth.RequestInfo) {
			requests <- info
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer authorization.Close()
	grants, err = workspace.NewGrants(authorization.OwnerSubject())
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()

	verifier, err := NewStaticJWTVerifier(authorization.PublicKey(), authorization.KeyID())
	if err != nil {
		t.Fatal(err)
	}
	protected, err := NewOAuthHandler(OAuthConfig{
		ResourceURL:     testResource,
		Issuer:          "https://signalspace.example",
		OwnerSubject:    authorization.OwnerSubject(),
		workspaceWriter: grants,
	}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", protected)
	mux.Handle(metadataPath, protected)
	mux.Handle(metadataPath+"/mcp", protected)
	mux.Handle("/", authorization.Handler())
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	registerBody := `{"client_name":"OAuth write harness","redirect_uris":["` + oauthWriteCallback + `"],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`
	registered := oauthWriteHTTP(t, server, http.MethodPost, "/register", "application/json", registerBody, "", "")
	if registered.status != http.StatusCreated {
		t.Fatalf("client registration failed: %d %s", registered.status, registered.body)
	}
	registeredJSON := oauthWriteJSON(t, registered)
	clientID, ok := registeredJSON["client_id"].(string)
	if !ok || clientID == "" {
		t.Fatalf("registration did not return client_id: %v", registeredJSON)
	}
	sessionID, err := grants.GrantWithScopes(root, clientID, workspace.ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}

	verifierValue := strings.Repeat("v", 43)
	challengeDigest := sha256.Sum256([]byte(verifierValue))
	state := strings.Repeat("s", 16)
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {oauthWriteCallback},
		"scope":                 {diagnosticScope + " " + workspaceWriteScope},
		"resource":              {testResource},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challengeDigest[:])},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	authorizeResponse := oauthWriteHTTP(t, server, http.MethodGet, "/authorize?"+query.Encode(), "", "", "", "")
	if authorizeResponse.status != http.StatusOK {
		t.Fatalf("authorize failed: %d %s", authorizeResponse.status, authorizeResponse.body)
	}
	csrfMatch := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindSubmatch(authorizeResponse.body)
	if len(csrfMatch) != 2 {
		t.Fatal("authorize response did not contain CSRF form value")
	}
	cookies := (&http.Response{Header: authorizeResponse.header}).Cookies()
	if len(cookies) != 1 || cookies[0].Name != "signalspace_auth" {
		t.Fatalf("authorize response did not contain the local session cookie: %v", cookies)
	}
	request := <-requests
	if request.ClientID != clientID || request.Scope != diagnosticScope+" "+workspaceWriteScope {
		t.Fatalf("OAuth request changed before consent: %+v", request)
	}
	if err := authorization.DecideTerminal(request.ID, true); err != nil {
		t.Fatal(err)
	}

	completeForm := url.Values{"request": {request.ID}, "csrf": {string(csrfMatch[1])}}
	completeResponse := oauthWriteHTTP(t, server, http.MethodPost, "/authorize/complete", "application/x-www-form-urlencoded", completeForm.Encode(), "", cookies[0].Name+"="+cookies[0].Value)
	if completeResponse.status != http.StatusSeeOther {
		t.Fatalf("authorize completion failed: %d %s", completeResponse.status, completeResponse.body)
	}
	redirect, err := url.Parse(completeResponse.header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	code := redirect.Query().Get("code")
	if code == "" || redirect.Query().Get("state") != state || redirect.Query().Get("iss") != "https://signalspace.example" {
		t.Fatalf("OAuth redirect claims are incomplete: %s", redirect)
	}

	tokenForm := url.Values{
		"grant_type":    {"authorization_code"},
		"resource":      {testResource},
		"code":          {code},
		"code_verifier": {verifierValue},
		"client_id":     {clientID},
		"redirect_uri":  {oauthWriteCallback},
	}
	tokenResponse := oauthWriteHTTP(t, server, http.MethodPost, "/token", "application/x-www-form-urlencoded", tokenForm.Encode(), "", "")
	if tokenResponse.status != http.StatusOK {
		t.Fatalf("token exchange failed: %d %s", tokenResponse.status, tokenResponse.body)
	}
	issued := oauthWriteJSON(t, tokenResponse)
	accessToken, ok := issued["access_token"].(string)
	if !ok || accessToken == "" || issued["scope"] != diagnosticScope+" "+workspaceWriteScope {
		t.Fatalf("token did not preserve exact approved scope: %v", issued)
	}
	claims := oauthWriteClaims(t, accessToken)
	if claims["iss"] != "https://signalspace.example" || claims["aud"] != testResource || claims["scope"] != diagnosticScope+" "+workspaceWriteScope || claims["client_id"] != clientID {
		t.Fatalf("issued token claims are not bound to the OAuth request: %v", claims)
	}
	identity, err := verifier.VerifyIdentity(context.Background(), accessToken, "https://signalspace.example", testResource, workspaceWriteScope, authorization.OwnerSubject())
	if err != nil || identity.ClientID != clientID {
		t.Fatalf("real verifier rejected issued write identity: %+v %v", identity, err)
	}
	if err := verifier.Verify(context.Background(), accessToken, "https://signalspace.example", testResource, diagnosticScope, authorization.OwnerSubject()); err != nil {
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
