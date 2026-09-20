package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
)

type requestFixture struct {
	mu        sync.Mutex
	item      auth.RequestSnapshot
	decisions int
	entered   chan struct{}
	release   chan struct{}
}

func newRequestFixture() *requestFixture {
	return &requestFixture{item: auth.RequestSnapshot{ID: strings.Repeat("a", 22), Version: 1, Status: "PENDING"}}
}

func (f *requestFixture) ListRequestSnapshots() []auth.RequestSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []auth.RequestSnapshot{f.item}
}

func (f *requestFixture) GetRequestSnapshot(id string) (auth.RequestSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != f.item.ID {
		return auth.RequestSnapshot{}, auth.ErrOAuthRequestNotFound
	}
	return f.item, nil
}

func (f *requestFixture) DecideAndSnapshot(id string, version int, decision string) (auth.RequestSnapshot, error) {
	if f.entered != nil {
		close(f.entered)
		<-f.release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != f.item.ID {
		return auth.RequestSnapshot{}, auth.ErrOAuthRequestNotFound
	}
	if f.item.Status != "PENDING" {
		return auth.RequestSnapshot{}, auth.ErrOAuthAlreadyDecided
	}
	if version != f.item.Version {
		return auth.RequestSnapshot{}, auth.ErrOAuthStaleRequest
	}
	if decision != "approve" && decision != "deny" {
		return auth.RequestSnapshot{}, auth.ErrOAuthInvalidDecision
	}
	f.decisions++
	f.item.Version++
	if decision == "approve" {
		f.item.Status = "APPROVED"
	} else {
		f.item.Status = "DENIED"
	}
	return f.item, nil
}

func authenticatedRequests(t *testing.T, gate *Gate, code string, fixture *requestFixture) (http.Handler, *http.Cookie, string) {
	t.Helper()
	handler := gate.HandlerWithRequests(fixture)
	bootstrap := adminRequest(handler, http.MethodGet, "/api/admin/v1/session", "", "")
	if bootstrap.Code != 200 {
		t.Fatalf("bootstrap failed: %d", bootstrap.Code)
	}
	csrf := adminResponse(t, bootstrap)["csrf_token"].(string)
	body, _ := json.Marshal(map[string]string{"pairing_code": code, "passphrase": testPassphrase})
	paired := adminRequest(handler, http.MethodPost, "/api/admin/v1/pair", string(body), csrf, responseCookie(t, bootstrap, bootstrapCookie))
	if paired.Code != http.StatusCreated {
		t.Fatalf("pair failed: %d %s", paired.Code, paired.Body.String())
	}
	return handler, responseCookie(t, paired, adminCookie), adminResponse(t, paired)["csrf_token"].(string)
}

func decisionPath(id string) string { return requestsPath + "/" + id + "/decision" }

func TestOAuthRequestsRequireOwnerCSRFAndExactPayload(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	fixture := newRequestFixture()
	handler := gate.HandlerWithRequests(fixture)
	if got := adminRequest(handler, http.MethodGet, requestsPath, "", ""); got.Code != 401 {
		t.Fatalf("unauthenticated list: %d", got.Code)
	}
	if got := adminRequest(handler, http.MethodPost, decisionPath(fixture.item.ID), `{"decision":"approve","expected_version":1}`, ""); got.Code != 401 {
		t.Fatalf("unauthenticated decision: %d", got.Code)
	}
	handler, cookie, csrf := authenticatedRequests(t, gate, code, fixture)
	list := adminRequest(handler, http.MethodGet, requestsPath, "", "", cookie)
	if list.Code != 200 || !strings.Contains(list.Body.String(), `"next_poll_after_ms":2000`) {
		t.Fatalf("authenticated list: %d %s", list.Code, list.Body.String())
	}
	if got := adminRequest(handler, http.MethodGet, requestsPath+"/"+fixture.item.ID, "", "", cookie); got.Code != 200 {
		t.Fatalf("authenticated detail: %d", got.Code)
	}
	if got := adminRequest(handler, http.MethodGet, requestsPath+"/missing", "", "", cookie); got.Code != 404 {
		t.Fatalf("unknown detail: %d", got.Code)
	}
	body := `{"decision":"approve","expected_version":1}`
	if got := adminRequest(handler, http.MethodPost, decisionPath(fixture.item.ID), body, "", cookie); got.Code != 403 {
		t.Fatalf("missing csrf accepted: %d", got.Code)
	}
	if got := adminRequest(handler, http.MethodPost, decisionPath(fixture.item.ID), body, "other-session", cookie); got.Code != 403 {
		t.Fatalf("invalid csrf accepted: %d", got.Code)
	}
	if got := adminRequest(handler, http.MethodPost, decisionPath(fixture.item.ID), `{"decision":"approve","expected_version":1,"client_id":"evil"}`, csrf, cookie); got.Code != 400 {
		t.Fatalf("untrusted client metadata accepted: %d", got.Code)
	}
	if got := adminRequest(handler, http.MethodPost, decisionPath(fixture.item.ID), `{"decision":"approve","expected_version":2}`, csrf, cookie); got.Code != 409 || !strings.Contains(got.Body.String(), "STALE_REQUEST") {
		t.Fatalf("stale version accepted: %d %s", got.Code, got.Body.String())
	}
	approved := adminRequest(handler, http.MethodPost, decisionPath(fixture.item.ID), body, csrf, cookie)
	if approved.Code != 200 || !strings.Contains(approved.Body.String(), `"status":"APPROVED"`) {
		t.Fatalf("decision failed: %d %s", approved.Code, approved.Body.String())
	}
	if got := adminRequest(handler, http.MethodPost, decisionPath(fixture.item.ID), body, csrf, cookie); got.Code != 409 || !strings.Contains(got.Body.String(), "ALREADY_DECIDED") {
		t.Fatalf("duplicate changed result: %d %s", got.Code, got.Body.String())
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.decisions != 1 {
		t.Fatalf("decision committed %d times", fixture.decisions)
	}
}

func TestOAuthRequestsRejectCrossOriginAndExpiredOwner(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	fixture := newRequestFixture()
	handler, cookie, csrf := authenticatedRequests(t, gate, code, fixture)
	crossOrigin := httptest.NewRequest(http.MethodPost, decisionPath(fixture.item.ID), strings.NewReader(`{"decision":"approve","expected_version":1}`))
	crossOrigin.Host = "localhost:7677"
	crossOrigin.Header.Set("Origin", "https://attacker.invalid")
	crossOrigin.Header.Set("Content-Type", "application/json")
	crossOrigin.Header.Set("X-CSRF-Token", csrf)
	crossOrigin.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, crossOrigin)
	if response.Code != 403 || response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("cross-origin decision accepted")
	}
	gate.mu.Lock()
	gate.now = func() time.Time { return time.Now().Add(absoluteTTL + time.Minute) }
	gate.mu.Unlock()
	if got := adminRequest(handler, http.MethodGet, requestsPath, "", "", cookie); got.Code != 401 {
		t.Fatalf("expired owner accessed requests: %d", got.Code)
	}
	if got := adminRequest(handler, http.MethodPost, decisionPath(fixture.item.ID), `{"decision":"approve","expected_version":1}`, csrf, cookie); got.Code != 401 {
		t.Fatalf("expired owner decided: %d", got.Code)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.decisions != 0 {
		t.Fatal("expired owner changed OAuth request")
	}
}

func TestOAuthDecisionAndLockAreSerialized(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	fixture := newRequestFixture()
	fixture.entered = make(chan struct{})
	fixture.release = make(chan struct{})
	handler, cookie, csrf := authenticatedRequests(t, gate, code, fixture)
	decisionDone := make(chan int, 1)
	go func() {
		result := adminRequest(handler, http.MethodPost, decisionPath(fixture.item.ID), `{"decision":"deny","expected_version":1}`, csrf, cookie)
		decisionDone <- result.Code
	}()
	select {
	case <-fixture.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("decision did not reach protected region")
	}
	lockDone := make(chan int, 1)
	go func() {
		result := adminRequest(handler, http.MethodPost, "/api/admin/v1/lock", `{}`, csrf, cookie)
		lockDone <- result.Code
	}()
	close(fixture.release)
	if code := <-decisionDone; code != 200 {
		t.Fatalf("decision result: %d", code)
	}
	if code := <-lockDone; code != 200 {
		t.Fatalf("lock result: %d", code)
	}
	if result := adminRequest(handler, http.MethodPost, decisionPath(fixture.item.ID), `{"decision":"approve","expected_version":2}`, csrf, cookie); result.Code != 401 {
		t.Fatalf("locked session reused: %d", result.Code)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.decisions != 1 || !errors.Is(auth.ErrOAuthAlreadyDecided, auth.ErrOAuthAlreadyDecided) {
		t.Fatal("unexpected decision count")
	}
}
