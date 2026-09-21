package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func startProgrammingAuth(t *testing.T, canGit, canTest *atomic.Bool) (*Server, http.Handler, <-chan RequestInfo) {
	t.Helper()
	requests := make(chan RequestInfo, 8)
	server, err := New(Config{
		ResourceURL: resourceURL,
		Issuer:      "https://signalspace.example",
		Scope:       scope,
		GitScope:    gitReviewScope,
		CanIssueGit: func(clientID string) bool { return clientID != "" && canGit.Load() },
		TestScope:   testRunScope,
		CanIssueTest: func(clientID string) bool {
			return clientID != "" && canTest.Load()
		},
		StateDir:  filepath.Join(t.TempDir(), "identity"),
		OnRequest: func(info RequestInfo) { requests <- info },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server, server.Handler(), requests
}

func TestProgrammingScopesMetadataCanonicalConsentAndBoundaryRevalidation(t *testing.T) {
	var canGit, canTest atomic.Bool
	canGit.Store(true)
	canTest.Store(true)
	server, handler, requests := startProgrammingAuth(t, &canGit, &canTest)
	client := register(t, handler)

	metadata := invoke(handler, http.MethodGet, "/.well-known/oauth-authorization-server", "", "", nil)
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), gitReviewScope) || !strings.Contains(metadata.Body.String(), testRunScope) {
		t.Fatalf("programming scopes were not advertised: %d %s", metadata.Code, metadata.Body.String())
	}
	var discovered struct {
		Scopes []string `json:"scopes_supported"`
	}
	if err := json.Unmarshal(metadata.Body.Bytes(), &discovered); err != nil {
		t.Fatal(err)
	}
	wantScopes := []string{scope, gitReviewScope, testRunScope}
	if !reflect.DeepEqual(discovered.Scopes, wantScopes) {
		t.Fatalf("unexpected programming scope order: got=%v want=%v", discovered.Scopes, wantScopes)
	}

	validScopes := []string{
		scope,
		scope + " " + gitReviewScope,
		scope + " " + testRunScope,
		scope + " " + gitReviewScope + " " + testRunScope,
	}
	for _, requested := range validScopes {
		result, _, _ := requestProgrammingConsent(t, handler, client, requested)
		if result.Code != http.StatusOK {
			t.Fatalf("canonical scope %q was rejected: %d %s", requested, result.Code, result.Body.String())
		}
		request := <-requests
		if request.Scope != requested {
			t.Fatalf("request changed canonical scope: got=%q want=%q", request.Scope, requested)
		}
		if err := server.DecideTerminal(request.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, requested := range []string{
		gitReviewScope,
		testRunScope + " " + scope,
		scope + " " + testRunScope + " " + gitReviewScope,
		scope + " " + gitReviewScope + " " + gitReviewScope,
		scope + "  " + gitReviewScope,
		scope + " signalspace:unknown",
	} {
		result, _, _ := requestProgrammingConsent(t, handler, client, requested)
		if result.Code != http.StatusBadRequest {
			t.Fatalf("invalid programming scope %q was accepted: %d", requested, result.Code)
		}
	}

	requested := scope + " " + gitReviewScope + " " + testRunScope
	result, cookie, csrf := requestProgrammingConsent(t, handler, client, requested)
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), "executar o teste predefinido") || !strings.Contains(result.Body.String(), "privilégios do usuário") || !strings.Contains(result.Body.String(), "O workspace não é sandbox") || !strings.Contains(result.Body.String(), "inspecionar status e diff Git") || !strings.Contains(result.Body.String(), "não autoriza commit ou push") {
		t.Fatalf("programming consent did not describe both capabilities: %d %s", result.Code, result.Body.String())
	}
	request := <-requests
	if err := server.DecideTerminal(request.ID, true); err != nil {
		t.Fatal(err)
	}
	canGit.Store(false)
	canTest.Store(false)
	if response := complete(handler, request.ID, csrf, cookie); response.Code != http.StatusForbidden {
		t.Fatalf("revoked programming grant was accepted at completion: %d", response.Code)
	}
	canGit.Store(true)
	canTest.Store(true)
	if response := complete(handler, request.ID, csrf, cookie); response.Code != http.StatusSeeOther {
		t.Fatalf("restored programming grant did not complete: %d", response.Code)
	}

	result, cookie, csrf = requestProgrammingConsent(t, handler, client, requested)
	request = <-requests
	if err := server.DecideTerminal(request.ID, true); err != nil {
		t.Fatal(err)
	}
	redirect := complete(handler, request.ID, csrf, cookie)
	code := redirect.Header().Get("Location")
	if redirect.Code != http.StatusSeeOther || code == "" {
		t.Fatalf("programming authorization did not return a code: %d %s", redirect.Code, redirect.Body.String())
	}
	target, err := url.Parse(code)
	if err != nil {
		t.Fatal(err)
	}
	canGit.Store(false)
	canTest.Store(false)
	if response := redeem(handler, client, target.Query().Get("code"), testVerifier, resourceURL); response.Code != http.StatusBadRequest {
		t.Fatalf("revoked programming grant was accepted at token exchange: %d", response.Code)
	}
}

func requestProgrammingConsent(t *testing.T, handler http.Handler, clientID, wantedScope string) (*httptest.ResponseRecorder, *http.Cookie, string) {
	t.Helper()
	result, cookie, csrf := requestWriteConsent(t, handler, clientID, wantedScope)
	return result, cookie, csrf
}
