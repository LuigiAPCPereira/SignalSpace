package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
)

const resourceURL = "https://signalspace.example/mcp"
const callback = "https://chatgpt.com/connector_platform_oauth_redirect"
const scope = "signalspace:diagnostic"
const testVerifier = "valid-verifier-with-enough-length-to-meet-pkce-requirements-2026"

func startAuth(t *testing.T) (*Server, http.Handler, <-chan RequestInfo) {
	t.Helper()
	events := make(chan RequestInfo, 5)
	dir := filepath.Join(t.TempDir(), "state")
	s, err := New(Config{ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope, StateDir: dir, OnRequest: func(event RequestInfo) { events <- event }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	verifier, err := mcp.NewStaticJWTVerifier(s.PublicKey(), s.KeyID())
	if err != nil {
		t.Fatal(err)
	}
	protected, err := mcp.NewOAuthHandler(mcp.OAuthConfig{ResourceURL: resourceURL, Issuer: "https://signalspace.example", OwnerSubject: s.OwnerSubject()}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", protected)
	for _, path := range []string{"/.well-known/oauth-authorization-server", "/oauth/jwks", "/register", "/authorize", "/authorize/complete", "/token"} {
		mux.Handle(path, s.Handler())
	}
	return s, mux, events
}
func invoke(h http.Handler, method, path, body, contentType string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = "signalspace.example"
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func register(t *testing.T, h http.Handler) string {
	t.Helper()
	b := fmt.Sprintf(`{"client_name":"ChatGPT","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none","scope":"%s"}`, callback, scope)
	w := invoke(h, "POST", "/register", b, "application/json", nil)
	if w.Code != 201 {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	var data struct {
		ClientID string `json:"client_id"`
	}
	if json.Unmarshal(w.Body.Bytes(), &data) != nil || data.ClientID == "" {
		t.Fatal("missing client ID")
	}
	return data.ClientID
}
func requestConsent(t *testing.T, h http.Handler, clientID string) (*httptest.ResponseRecorder, *http.Cookie, string) {
	t.Helper()
	sum := sha256.Sum256([]byte(testVerifier))
	q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {callback}, "scope": {scope}, "resource": {resourceURL}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "state": {"state-random-identifier-for-test"}}
	w := invoke(h, "GET", "/authorize?"+q.Encode(), "", "", nil)
	if w.Code != 200 {
		t.Fatalf("authorize %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatal("missing secure session cookie")
	}
	csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(w.Body.String())
	if len(csrf) != 2 {
		t.Fatal("missing CSRF token")
	}
	return w, cookies[0], csrf[1]
}
func complete(h http.Handler, id, csrf string, cookie *http.Cookie) *httptest.ResponseRecorder {
	f := url.Values{"request": {id}, "csrf": {csrf}}
	return invoke(h, "POST", "/authorize/complete", f.Encode(), "application/x-www-form-urlencoded", cookie)
}
func redeem(h http.Handler, clientID, code, verifier, resource string) *httptest.ResponseRecorder {
	f := url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "redirect_uri": {callback}, "code": {code}, "code_verifier": {verifier}, "resource": {resource}}
	return invoke(h, "POST", "/token", f.Encode(), "application/x-www-form-urlencoded", nil)
}

func TestCompleteOAuthFlowAndSingleUse(t *testing.T) {
	s, h, events := startAuth(t)
	id := register(t, h)
	metadata := invoke(h, "GET", "/.well-known/oauth-authorization-server", "", "", nil)
	if metadata.Code != 200 || !strings.Contains(metadata.Body.String(), `"registration_endpoint"`) {
		t.Fatal("missing discovery")
	}
	jwks := invoke(h, "GET", "/oauth/jwks", "", "", nil)
	if jwks.Code != 200 || !strings.Contains(jwks.Body.String(), s.KeyID()) {
		t.Fatal("missing signing key")
	}
	_, cookie, csrf := requestConsent(t, h, id)
	p := <-events
	if w := complete(h, p.ID, csrf, cookie); w.Code != 409 {
		t.Fatalf("approved without owner: %d", w.Code)
	}
	if w := complete(h, p.ID, "invalid", cookie); w.Code != 403 {
		t.Fatalf("CSRF accepted: %d", w.Code)
	}
	if w := complete(h, p.ID, csrf, nil); w.Code != 403 {
		t.Fatalf("missing cookie accepted: %d", w.Code)
	}
	if err := s.Approve(p.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Approve(p.ID, true); err == nil {
		t.Fatal("duplicate owner approval")
	}
	redirect := complete(h, p.ID, csrf, cookie)
	if redirect.Code != 303 {
		t.Fatalf("redirect %d %s", redirect.Code, redirect.Body.String())
	}
	target, err := url.Parse(redirect.Header().Get("Location"))
	if err != nil || target.Host != "chatgpt.com" || target.Query().Get("iss") != "https://signalspace.example" || target.Query().Get("state") != "state-random-identifier-for-test" {
		t.Fatalf("invalid callback: %v", target)
	}
	code := target.Query().Get("code")
	if code == "" {
		t.Fatal("missing authorization code")
	}
	if w := complete(h, p.ID, csrf, cookie); w.Code != 403 {
		t.Fatalf("approval reused: %d", w.Code)
	}
	wrong := redeem(h, id, code, testVerifier, resourceURL+"/other")
	if wrong.Code != 400 {
		t.Fatal("wrong resource accepted")
	}
	if w := redeem(h, id, code, testVerifier, resourceURL); w.Code != 400 {
		t.Fatal("authorization code replay accepted")
	}
	_, cookie, csrf = requestConsent(t, h, id)
	p = <-events
	if err := s.Approve(p.ID, true); err != nil {
		t.Fatal(err)
	}
	target, _ = url.Parse(complete(h, p.ID, csrf, cookie).Header().Get("Location"))
	code = target.Query().Get("code")
	tokenResponse := redeem(h, id, code, testVerifier, resourceURL)
	if tokenResponse.Code != 200 {
		t.Fatalf("token exchange %d %s", tokenResponse.Code, tokenResponse.Body.String())
	}
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Expires     int    `json:"expires_in"`
	}
	if json.Unmarshal(tokenResponse.Body.Bytes(), &token) != nil || token.TokenType != "Bearer" || token.Expires != 900 {
		t.Fatal("invalid token contract")
	}
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"connection_diagnostic","arguments":{}}}`))
	r.Host = "signalspace.example"
	r.Header.Set("Authorization", "Bearer "+token.AccessToken)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("MCP-Protocol-Version", "2025-06-18")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `\"connected\":true`) {
		t.Fatalf("token not accepted by MCP: %d %s", w.Code, w.Body.String())
	}
	if w := redeem(h, id, code, testVerifier, resourceURL); w.Code != 400 {
		t.Fatal("token issued on replay")
	}
}

func TestBoundariesAndDenial(t *testing.T) {
	s, h, events := startAuth(t)
	badRedirect := `{"client_name":"evil","redirect_uris":["https://evil.example/callback"]}`
	if w := invoke(h, "POST", "/register", badRedirect, "application/json", nil); w.Code != 400 {
		t.Fatal("registered attacker redirect")
	}
	if w := invoke(h, "POST", "/register", `{"client_name":"evil\nname","redirect_uris":["`+callback+`"]}`, "application/json", nil); w.Code != 400 {
		t.Fatal("log injection accepted")
	}
	if w := invoke(h, "POST", "/register", `{"client_name":"ChatGPT","redirect_uris":["`+callback+`"]}`, "text/plain", nil); w.Code != 415 {
		t.Fatalf("unexpected media type: %d", w.Code)
	}
	id := register(t, h)
	_, cookie, csrf := requestConsent(t, h, id)
	p := <-events
	if err := s.Approve(p.ID, false); err != nil {
		t.Fatal(err)
	}
	if w := complete(h, p.ID, csrf, cookie); w.Code != 403 {
		t.Fatal("denied request accepted")
	}
	if w := invoke(h, "POST", "/approve", p.ID, "application/json", nil); w.Code != 404 {
		t.Fatalf("public approval endpoint exists: %d", w.Code)
	}
	if err := s.Approve(p.ID, true); err == nil {
		t.Fatal("denial reversed")
	}
	_, cookie, csrf = requestConsent(t, h, id)
	p = <-events
	s.mu.Lock()
	state := s.pending[p.ID]
	state.Expires = time.Now().Add(-time.Second)
	s.pending[p.ID] = state
	s.mu.Unlock()
	if err := s.Approve(p.ID, true); err == nil {
		t.Fatal("expired approval accepted")
	}
	if w := complete(h, p.ID, csrf, cookie); w.Code != 403 {
		t.Fatal("expired request accepted")
	}
	if w := invoke(h, "GET", "/.well-known/oauth-authorization-server", "", "", nil); w.Code != 200 {
		t.Fatal("missing metadata")
	}
}
func TestRejectInvalidConfiguration(t *testing.T) {
	for _, cfg := range []Config{{}, {ResourceURL: resourceURL, Issuer: "https://other.example", Scope: scope}, {ResourceURL: "http://signalspace.example/mcp", Issuer: "http://signalspace.example", Scope: scope}, {ResourceURL: resourceURL + "?", Issuer: "https://signalspace.example", Scope: scope}} {
		if _, err := New(cfg); err == nil {
			t.Fatalf("accepted config %+v", cfg)
		}
	}
}
