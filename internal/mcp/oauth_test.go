package mcp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testResource = "https://signalspace.example/mcp"
const testIssuer = "https://identity.example/"

func makeAccessToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT", "kid": "test-key"})
	payload, _ := json.Marshal(claims)
	message := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return message + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func defaultClaims() map[string]any {
	return map[string]any{
		"iss": testIssuer, "sub": "owner-test", "aud": testResource,
		"exp": time.Now().Add(time.Hour).Unix(), "scope": diagnosticScope,
	}
}

func setupOAuth(t *testing.T) (*rsa.PrivateKey, *JWKSVerifier, *httptest.Server, *httptest.Server) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kid": "test-key", "kty": "RSA", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}),
		}}})
	}))
	t.Cleanup(jwks.Close)
	verifier, err := NewJWKSVerifier(jwks.URL)
	if err != nil {
		t.Fatal(err)
	}
	verifier.client = jwks.Client()
	verifier.client.Timeout = 3 * time.Second
	verifier.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	handler, err := NewOAuthHandler(OAuthConfig{ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: "owner-test"}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return key, verifier, jwks, server
}

func oauthRequest(t *testing.T, server *httptest.Server, method, path, token, body string, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "signalspace.example"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
		} else {
			req.Header.Set(k, v)
		}
	}
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if len(data) > 0 && strings.Contains(res.Header.Get("Content-Type"), "application/json") && json.Unmarshal(data, &object) != nil {
		t.Fatalf("invalid JSON status %d", res.StatusCode)
	}
	res.Body = io.NopCloser(bytes.NewReader(data))
	return res, object
}

func TestOAuthMetadataAndBoundary(t *testing.T) {
	_, _, _, server := setupOAuth(t)
	for _, path := range []string{metadataPath, metadataPath + "/mcp"} {
		res, data := oauthRequest(t, server, http.MethodGet, path, "", "", nil)
		if res.StatusCode != 200 || data["resource"] != testResource {
			t.Fatalf("metadata: status=%d body=%v", res.StatusCode, data)
		}
		servers := data["authorization_servers"].([]any)
		if servers[0] != testIssuer {
			t.Fatalf("issuer mismatch: %v", servers)
		}
	}
	res, _ := oauthRequest(t, server, http.MethodPost, "/mcp", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if res.StatusCode != 401 || !strings.Contains(res.Header.Get("WWW-Authenticate"), `resource_metadata="https://signalspace.example/.well-known/oauth-protected-resource"`) {
		t.Fatalf("missing token: status=%d challenge=%s", res.StatusCode, res.Header.Get("WWW-Authenticate"))
	}
	for _, tc := range []struct {
		method, path string
		headers      map[string]string
		want         int
	}{
		{http.MethodGet, metadataPath, map[string]string{"Host": "attacker.example"}, 403},
		{http.MethodGet, metadataPath, map[string]string{"Origin": "https://attacker.example"}, 403},
		{http.MethodPost, metadataPath, nil, 405},
		{http.MethodGet, metadataPath + "?resource=other", nil, 404},
		{http.MethodGet, "/unknown", nil, 404},
	} {
		res, _ := oauthRequest(t, server, tc.method, tc.path, "", "", tc.headers)
		if res.StatusCode != tc.want {
			t.Errorf("path %s got %d want %d", tc.path, res.StatusCode, tc.want)
		}
	}
}

func TestOAuthJWTClaimsAndTool(t *testing.T) {
	key, _, _, server := setupOAuth(t)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	valid := makeAccessToken(t, key, defaultClaims())
	res, result := oauthRequest(t, server, http.MethodPost, "/mcp", valid, body, nil)
	if res.StatusCode != 200 {
		t.Fatalf("valid JWT status=%d", res.StatusCode)
	}
	tool := result["result"].(map[string]any)["tools"].([]any)[0].(map[string]any)
	schemes := tool["securitySchemes"].([]any)
	if schemes[0].(map[string]any)["type"] != "oauth2" {
		t.Fatalf("missing tool OAuth declaration: %v", tool)
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", valid, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"connection_diagnostic","arguments":{}}}`, nil)
	if res.StatusCode != 200 || result["error"] != nil {
		t.Fatalf("diagnostic rejected: %d %v", res.StatusCode, result)
	}
	for name, mutate := range map[string]func(map[string]any){
		"issuer":        func(c map[string]any) { c["iss"] = "https://attacker.example/" },
		"audience":      func(c map[string]any) { c["aud"] = "https://other.example/mcp" },
		"expired":       func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() },
		"not_yet_valid": func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() },
		"subject":       func(c map[string]any) { c["sub"] = "" },
		"other_owner":   func(c map[string]any) { c["sub"] = "other-user" },
	} {
		t.Run(name, func(t *testing.T) {
			claims := defaultClaims()
			mutate(claims)
			res, _ := oauthRequest(t, server, http.MethodPost, "/mcp", makeAccessToken(t, key, claims), body, nil)
			if res.StatusCode != 401 {
				t.Fatalf("invalid claim accepted: %d", res.StatusCode)
			}
		})
	}
	claims := defaultClaims()
	claims["scope"] = "profile:read"
	res, _ = oauthRequest(t, server, http.MethodPost, "/mcp", makeAccessToken(t, key, claims), body, nil)
	if res.StatusCode != 403 || !strings.Contains(res.Header.Get("WWW-Authenticate"), "insufficient_scope") {
		t.Fatalf("missing scope accepted: %d", res.StatusCode)
	}
	res, _ = oauthRequest(t, server, http.MethodPost, "/mcp", valid+"invalid", body, nil)
	if res.StatusCode != 401 {
		t.Fatalf("tampered token accepted: %d", res.StatusCode)
	}
	res, _ = oauthRequest(t, server, http.MethodPost, "/mcp", strings.Repeat("X", maxTokenBytes+1), body, nil)
	if res.StatusCode != 401 {
		t.Fatalf("oversized token accepted: %d", res.StatusCode)
	}
	// O servidor não pode usar credenciais do diagnóstico local como fallback OAuth.
	res, _ = oauthRequest(t, server, http.MethodPost, "/mcp", testToken, body, nil)
	if res.StatusCode != 401 {
		t.Fatalf("local credential accepted as OAuth: %d", res.StatusCode)
	}
}

func TestOAuthJWKSUnavailableFailsClosed(t *testing.T) {
	key, verifier, keys, server := setupOAuth(t)
	token := makeAccessToken(t, key, defaultClaims())
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	res, _ := oauthRequest(t, server, http.MethodPost, "/mcp", token, body, nil)
	if res.StatusCode != 200 {
		t.Fatalf("initial verification failed: %d", res.StatusCode)
	}
	verifier.mu.Lock()
	verifier.expires = time.Now().Add(-time.Second)
	verifier.mu.Unlock()
	keys.Close()
	res, _ = oauthRequest(t, server, http.MethodPost, "/mcp", token, body, nil)
	if res.StatusCode != 401 {
		t.Fatalf("expired JWKS cache used: %d", res.StatusCode)
	}
}

func TestOAuthConfigRejectsUnsafeSetup(t *testing.T) {
	for _, cfg := range []OAuthConfig{
		{ResourceURL: "http://signalspace.example/mcp", Issuer: testIssuer, OwnerSubject: "owner-test"},
		{ResourceURL: "https://signalspace.example/mcp?x=1", Issuer: testIssuer, OwnerSubject: "owner-test"},
		{ResourceURL: "https://signalspace.example/other", Issuer: testIssuer, OwnerSubject: "owner-test"},
		{ResourceURL: testResource, Issuer: "http://identity.example", OwnerSubject: "owner-test"},
		{ResourceURL: testResource, Issuer: "https://identity.example/#evil", OwnerSubject: "owner-test"},
	} {
		if _, err := NewOAuthHandler(cfg, fakeVerifier{}); err == nil {
			t.Errorf("accepted invalid OAuth config: %+v", cfg)
		}
	}
	for _, owner := range []string{"", " owner-test", "owner-test ", "owner\nother"} {
		if _, err := NewOAuthHandler(OAuthConfig{ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: owner}, fakeVerifier{}); err == nil {
			t.Errorf("accepted invalid owner subject %q", owner)
		}
	}
	if _, err := NewOAuthHandler(OAuthConfig{ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: "owner-test"}, nil); err == nil {
		t.Fatal("accepted nil OAuth verifier")
	}
	if _, err := NewJWKSVerifier("http://identity.example/jwks"); err == nil {
		t.Fatal("accepted plaintext JWKS URL")
	}
	if _, err := NewJWKSVerifier("https://user:secret@identity.example/jwks"); err == nil {
		t.Fatal("accepted JWKS URL containing credentials")
	}
	if _, err := NewJWKSVerifier("https://identity.example/jwks"); err != nil {
		t.Fatal(fmt.Errorf("valid JWKS URL: %w", err))
	}
}

type fakeVerifier struct{}

func (fakeVerifier) Verify(_ context.Context, _, _, _, _, _ string) error { return nil }
