package auth

import (
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
)

const writeScope = "signalspace:workspace.write"

func startWriteAuth(t *testing.T, canWrite func(string) bool) (*Server, http.Handler, <-chan RequestInfo) {
	t.Helper()
	requests := make(chan RequestInfo, 8)
	s, err := New(Config{
		ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope,
		WriteScope: writeScope, CanIssueWrite: canWrite,
		StateDir:  filepath.Join(t.TempDir(), "identity"),
		OnRequest: func(info RequestInfo) { requests <- info },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, s.Handler(), requests
}

func requestWriteConsent(t *testing.T, handler http.Handler, clientID, wantedScope string) (*httptest.ResponseRecorder, *http.Cookie, string) {
	t.Helper()
	digest := sha256.Sum256([]byte(testVerifier))
	params := url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {callback},
		"scope": {wantedScope}, "resource": {resourceURL},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(digest[:])},
		"code_challenge_method": {"S256"}, "state": {"state-random-identifier-for-test"},
	}
	result := invoke(handler, "GET", "/authorize?"+params.Encode(), "", "", nil)
	if result.Code != http.StatusOK {
		return result, nil, ""
	}
	cookies := result.Result().Cookies()
	csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(result.Body.String())
	if len(cookies) != 1 || len(csrf) != 2 {
		t.Fatal("write consent missing CSRF cookie or form")
	}
	return result, cookies[0], csrf[1]
}

func TestWriteScopeRequiresExplicitOptInAndRevocationAtOAuthBoundaries(t *testing.T) {
	var allowed atomic.Bool
	server, handler, requests := startWriteAuth(t, func(clientID string) bool {
		return allowed.Load() && clientID != ""
	})
	client := register(t, handler)

	metadata := invoke(handler, "GET", "/.well-known/oauth-authorization-server", "", "", nil)
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), writeScope) {
		t.Fatal("experimental write scope was not advertised by its explicit configuration")
	}

	wanted := scope + " " + writeScope
	for _, rejected := range []string{
		writeScope,
		writeScope + " " + scope,
		wanted + " " + writeScope,
		"signalspace:workspace.unknown",
		"signalspace:test.run",
	} {
		result, _, _ := requestWriteConsent(t, handler, client, rejected)
		if result.Code != http.StatusBadRequest {
			t.Fatalf("accepted unsupported write scope %q: %d", rejected, result.Code)
		}
	}

	result, _, _ := requestWriteConsent(t, handler, client, wanted)
	if result.Code != http.StatusForbidden {
		t.Fatalf("write scope without local grant accepted: %d", result.Code)
	}
	select {
	case <-requests:
		t.Fatal("created OAuth write request without local grant")
	default:
	}

	allowed.Store(true)
	result, cookie, csrf := requestWriteConsent(t, handler, client, wanted)
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), "modificar arquivos") || strings.Contains(result.Body.String(), "somente leitura") {
		t.Fatalf("write consent was not explicit: %d %s", result.Code, result.Body.String())
	}
	request := <-requests
	if request.Scope != wanted || request.ClientID != client {
		t.Fatalf("write request changed at authorization boundary: %+v", request)
	}
	if err := server.DecideTerminal(request.ID, true); err != nil {
		t.Fatal(err)
	}

	// Revogar entre a aprovação e a conclusão não consome o pedido nem emite código.
	allowed.Store(false)
	if result := complete(handler, request.ID, csrf, cookie); result.Code != http.StatusForbidden {
		t.Fatalf("issued code after write grant revocation: %d", result.Code)
	}
	server.mu.Lock()
	_, pending := server.pending[request.ID]
	server.mu.Unlock()
	if !pending {
		t.Fatal("revoked write request was consumed")
	}

	allowed.Store(true)
	redirect := complete(handler, request.ID, csrf, cookie)
	if redirect.Code != http.StatusSeeOther {
		t.Fatalf("approved write consent failed after grant restoration: %d", redirect.Code)
	}
	target, err := url.Parse(redirect.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	code := target.Query().Get("code")
	if code == "" || target.Query().Get("state") == "" || target.Query().Get("iss") != server.config.Issuer {
		t.Fatalf("authorization redirect lost OAuth response fields: %s", target)
	}

	// Revogar antes de /token também impede a emissão e o código não pode ser reutilizado.
	allowed.Store(false)
	if response := redeem(handler, client, code, testVerifier, resourceURL); response.Code != http.StatusBadRequest {
		t.Fatalf("issued token after write grant revocation: %d", response.Code)
	}
	allowed.Store(true)
	if response := redeem(handler, client, code, testVerifier, resourceURL); response.Code != http.StatusBadRequest {
		t.Fatalf("revoked authorization code was reusable: %d", response.Code)
	}
}

func TestWriteScopeIsAbsentFromDefaultAndRejectsUnsafeConfiguration(t *testing.T) {
	_, handler, _ := startAuth(t)
	client := register(t, handler)
	metadata := invoke(handler, "GET", "/.well-known/oauth-authorization-server", "", "", nil)
	if strings.Contains(metadata.Body.String(), writeScope) || strings.Contains(metadata.Body.String(), "signalspace:test.run") {
		t.Fatal("default OAuth metadata advertised an unpublished programming scope")
	}
	params := scope + " " + writeScope
	if result, _, _ := requestWriteConsent(t, handler, client, params); result.Code != http.StatusBadRequest {
		t.Fatalf("default issuer accepted write scope: %d", result.Code)
	}
	if result, _, _ := requestWriteConsent(t, handler, client, scope+" signalspace:test.run"); result.Code != http.StatusBadRequest {
		t.Fatalf("default issuer accepted test execution scope: %d", result.Code)
	}

	for _, bad := range []Config{
		{ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope, WriteScope: writeScope, StateDir: t.TempDir()},
		{ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope, CanIssueWrite: func(string) bool { return true }, StateDir: t.TempDir()},
		{ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope, WriteScope: "untrusted:scope", CanIssueWrite: func(string) bool { return true }, StateDir: t.TempDir(), OnRequest: func(RequestInfo) {}},
		{ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope, WriteScope: writeScope, CanIssueWrite: func(string) bool { return true }, StateDir: t.TempDir()},
	} {
		if server, err := New(bad); err == nil {
			_ = server.Close()
			t.Fatal("unsafe write scope configuration accepted")
		}
	}
}

func TestWriteScopeMetadataAndTokenScopeAreExact(t *testing.T) {
	allowed := true
	s, handler, _ := startWriteAuth(t, func(string) bool { return allowed })
	client := register(t, handler)
	metadata := invoke(handler, "GET", "/.well-known/oauth-authorization-server", "", "", nil)
	var item struct {
		Scopes []string `json:"scopes_supported"`
	}
	if err := json.Unmarshal(metadata.Body.Bytes(), &item); err != nil || len(item.Scopes) != 2 || item.Scopes[0] != scope || item.Scopes[1] != writeScope {
		t.Fatalf("unexpected write metadata scopes: %s", metadata.Body.String())
	}
	if client == "" || s.OwnerSubject() == "" {
		t.Fatal("write test fixture did not initialize identity")
	}
}
