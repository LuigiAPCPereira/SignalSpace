package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
)

const readScope = "signalspace:workspace.read"

func startReadAuth(t *testing.T, canRead func(string) bool) (*Server, http.Handler, <-chan RequestInfo) {
	t.Helper()
	requests := make(chan RequestInfo, 8)
	s, err := New(Config{
		ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope,
		ReadScope: readScope, CanIssueRead: canRead,
		StateDir:  filepath.Join(t.TempDir(), "identity"),
		OnRequest: func(info RequestInfo) { requests <- info },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, s.Handler(), requests
}

func requestReadConsent(t *testing.T, handler http.Handler, clientID, wantedScope string) (*httptest.ResponseRecorder, *http.Cookie, string) {
	t.Helper()
	digest := sha256.Sum256([]byte(testVerifier))
	params := url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {callback},
		"scope": {wantedScope}, "resource": {resourceURL},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(digest[:])},
		"code_challenge_method": {"S256"}, "state": {"state-random-identifier-for-test"},
	}
	result := invoke(handler, "GET", "/authorize?"+params.Encode(), "", "", nil)
	if result.Code != 200 {
		return result, nil, ""
	}
	cookies := result.Result().Cookies()
	csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(result.Body.String())
	if len(cookies) != 1 || len(csrf) != 2 {
		t.Fatal("consent flow missing CSRF cookie or form")
	}
	return result, cookies[0], csrf[1]
}

func TestReadScopeRequiresLocalGrantAndSeparateOAuthConsent(t *testing.T) {
	var allowed atomic.Bool
	client := ""
	server, handler, requests := startReadAuth(t, func(id string) bool {
		return allowed.Load() && id == client
	})
	client = register(t, handler)
	otherClient := register(t, handler)
	metadata := invoke(handler, "GET", "/.well-known/oauth-authorization-server", "", "", nil)
	if metadata.Code != 200 || !strings.Contains(metadata.Body.String(), readScope) {
		t.Fatal("read scope not advertised when explicitly configured")
	}
	wanted := scope + " " + readScope
	for _, rejected := range []string{readScope, scope + " " + scope, readScope + " " + scope, wanted + " " + readScope} {
		result, _, _ := requestReadConsent(t, handler, client, rejected)
		if result.Code != 400 {
			t.Fatalf("accepted unsupported scope %q: %d", rejected, result.Code)
		}
	}
	result, _, _ := requestReadConsent(t, handler, client, wanted)
	if result.Code != 403 {
		t.Fatalf("read scope before workspace grant accepted: %d", result.Code)
	}
	select {
	case <-requests:
		t.Fatal("created an OAuth request without workspace consent")
	default:
	}

	// A concessão local não dispensa a autorização OAuth separada.
	allowed.Store(true)
	if response, _, _ := requestReadConsent(t, handler, otherClient, wanted); response.Code != 403 {
		t.Fatalf("different OAuth client reused grant: %d", response.Code)
	}
	result, cookie, csrf := requestReadConsent(t, handler, client, wanted)
	if result.Code != 200 || !strings.Contains(result.Body.String(), "Permissão adicional") || !strings.Contains(result.Body.String(), readScope) || !strings.Contains(result.Body.String(), client) {
		t.Fatalf("read consent not explicit: %d %s", result.Code, result.Body.String())
	}
	req := <-requests
	if req.Scope != wanted || req.ClientID != client {
		t.Fatalf("requested scope not preserved: %q", req.Scope)
	}
	if err := server.Approve(req.ID, true); err != nil {
		t.Fatal(err)
	}
	// Revogar durante o consentimento deve impedir a emissão do código.
	allowed.Store(false)
	if result := complete(handler, req.ID, csrf, cookie); result.Code != 403 {
		t.Fatalf("issued code after revoke: %d", result.Code)
	}

	allowed.Store(true)
	_, cookie, csrf = requestReadConsent(t, handler, client, wanted)
	req = <-requests
	if err := server.Approve(req.ID, true); err != nil {
		t.Fatal(err)
	}
	redirect := complete(handler, req.ID, csrf, cookie)
	if redirect.Code != 303 {
		t.Fatalf("consent failed: %d", redirect.Code)
	}
	target, err := url.Parse(redirect.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	code := target.Query().Get("code")
	allowed.Store(false)
	if response := redeem(handler, client, code, testVerifier, resourceURL); response.Code != 400 {
		t.Fatalf("issued token after revoke: %d", response.Code)
	}
	// Código negado é consumido e não pode ser reutilizado após nova concessão.
	allowed.Store(true)
	if response := redeem(handler, client, code, testVerifier, resourceURL); response.Code != 400 {
		t.Fatalf("reused revoked code: %d", response.Code)
	}

	_, cookie, csrf = requestReadConsent(t, handler, client, wanted)
	req = <-requests
	if err := server.Approve(req.ID, true); err != nil {
		t.Fatal(err)
	}
	redirect = complete(handler, req.ID, csrf, cookie)
	target, _ = url.Parse(redirect.Header().Get("Location"))
	token := redeem(handler, client, target.Query().Get("code"), testVerifier, resourceURL)
	if token.Code != 200 {
		t.Fatalf("approved token denied: %d", token.Code)
	}
	var issued struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
	}
	if err := json.Unmarshal(token.Body.Bytes(), &issued); err != nil || issued.Scope != wanted || issued.AccessToken == "" {
		t.Fatalf("unexpected token scope: %q %v", issued.Scope, err)
	}
	verifier, err := mcp.NewStaticJWTVerifier(server.PublicKey(), server.KeyID())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := verifier.VerifyIdentity(context.Background(), issued.AccessToken, server.config.Issuer, resourceURL, readScope, server.OwnerSubject())
	if err != nil || identity.ClientID != client {
		t.Fatalf("read token identity missing: %+v %v", identity, err)
	}
	if err := verifier.Verify(context.Background(), issued.AccessToken, server.config.Issuer, resourceURL, scope, server.OwnerSubject()); err != nil {
		t.Fatalf("read token lost diagnostic scope: %v", err)
	}
}

func TestReadScopeDisabledByDefaultAndRejectsMisconfiguration(t *testing.T) {
	_, handler, _ := startAuth(t)
	client := register(t, handler)
	metadata := invoke(handler, "GET", "/.well-known/oauth-authorization-server", "", "", nil)
	if strings.Contains(metadata.Body.String(), readScope) {
		t.Fatal("read scope advertised in diagnostic-only configuration")
	}
	if result, _, _ := requestReadConsent(t, handler, client, scope+" "+readScope); result.Code != 400 {
		t.Fatalf("read scope accepted without explicit configuration: %d", result.Code)
	}
	for _, bad := range []Config{
		{ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope, ReadScope: readScope, StateDir: t.TempDir()},
		{ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope, CanIssueRead: func(string) bool { return true }, StateDir: t.TempDir()},
		{ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope, ReadScope: "untrusted:scope", CanIssueRead: func(string) bool { return true }, StateDir: t.TempDir()},
		{ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope, ReadScope: readScope, CanIssueRead: func(string) bool { return true }, StateDir: t.TempDir()},
	} {
		if s, err := New(bad); err == nil {
			_ = s.Close()
			t.Fatal("unsafe read scope configuration accepted")
		}
	}
}
