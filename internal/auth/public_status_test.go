package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func statusCall(h http.Handler, id string, cookie *http.Cookie) *httptest.ResponseRecorder {
	return invoke(h, http.MethodGet, "/authorize/status?request_id="+id, "", "", cookie)
}

func statusValue(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPublicStatusOnlyRevealsOwnRequestAndNeverApproves(t *testing.T) {
	s, h, events := startAuth(t)
	clientID := register(t, h)
	_, firstCookie, csrf := requestConsent(t, h, clientID)
	first := <-events
	_, otherCookie, _ := requestConsent(t, h, clientID)
	other := <-events
	status := s.PublicStatusHandler()
	if w := statusCall(status, first.ID, nil); w.Code != http.StatusForbidden {
		t.Fatalf("anonymous status: %d", w.Code)
	}
	if w := statusCall(status, first.ID, otherCookie); w.Code != http.StatusForbidden {
		t.Fatalf("other request cookie accessed status: %d", w.Code)
	}
	if w := statusCall(status, other.ID, firstCookie); w.Code != http.StatusForbidden {
		t.Fatalf("other request ID accessed status: %d", w.Code)
	}
	if w := statusCall(status, "not-a-valid-id", firstCookie); w.Code != http.StatusBadRequest {
		t.Fatalf("malformed ID: %d", w.Code)
	}
	if w := statusCall(status, "ABCDEFGHIJKLMNOPQRSTUV", firstCookie); w.Code != http.StatusNotFound {
		t.Fatalf("unknown ID: %d", w.Code)
	}
	w := statusCall(status, first.ID, firstCookie)
	if w.Code != http.StatusOK || statusValue(t, w)["status"] != "PENDING" {
		t.Fatalf("pending state: %d %s", w.Code, w.Body.String())
	}
	for _, forbidden := range []string{"client_id", "redirect_uri", "scope", "csrf", "access_token", "session_id", clientID, csrf} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatalf("private value leaked in status response: %q", forbidden)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Access-Control-Allow-Origin") != "" || len(w.Result().Cookies()) != 0 {
		t.Fatal("unsafe public status headers or cookie")
	}
	if complete(h, first.ID, csrf, firstCookie).Code != http.StatusConflict {
		t.Fatal("status polling implicitly approved the request")
	}
	if err := s.Approve(first.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := statusValue(t, statusCall(status, first.ID, firstCookie))["status"]; got != "APPROVED" {
		t.Fatalf("approved status: %v", got)
	}
	if err := s.Approve(other.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := statusValue(t, statusCall(status, other.ID, otherCookie))["status"]; got != "DENIED" {
		t.Fatalf("denied status: %v", got)
	}
}

func TestPublicStatusRejectsCrossOriginAndExpiresWithoutCode(t *testing.T) {
	s, h, events := startAuth(t)
	clientID := register(t, h)
	_, cookie, csrf := requestConsent(t, h, clientID)
	pendingRequest := <-events
	status := s.PublicStatusHandler()
	r := httptest.NewRequest("GET", "/authorize/status?request_id="+pendingRequest.ID, nil)
	r.Host = "signalspace.example"
	r.Header.Set("Origin", "https://attacker.example")
	r.AddCookie(cookie)
	crossOrigin := httptest.NewRecorder()
	status.ServeHTTP(crossOrigin, r)
	if crossOrigin.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status: %d", crossOrigin.Code)
	}
	r = httptest.NewRequest("POST", "/authorize/status?request_id="+pendingRequest.ID, nil)
	r.Host = "signalspace.example"
	r.AddCookie(cookie)
	method := httptest.NewRecorder()
	status.ServeHTTP(method, r)
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("public mutation reached status: %d", method.Code)
	}
	s.mu.Lock()
	item := s.pending[pendingRequest.ID]
	item.Expires = time.Now().Add(-time.Second)
	s.pending[pendingRequest.ID] = item
	s.mu.Unlock()
	if got := statusValue(t, statusCall(status, pendingRequest.ID, cookie))["status"]; got != "EXPIRED" {
		t.Fatalf("expiry status: %v", got)
	}
	if w := complete(h, pendingRequest.ID, csrf, cookie); w.Code == http.StatusSeeOther {
		t.Fatal("expired request issued an authorization code")
	}
	// O resultado da expiração deve sobreviver ao POST negado para reconciliação.
	if w := statusCall(status, pendingRequest.ID, cookie); w.Code != http.StatusOK || statusValue(t, w)["status"] != "EXPIRED" {
		t.Fatalf("expiry tombstone missing: %d", w.Code)
	}
	s.mu.Lock()
	record := s.terminal[pendingRequest.ID]
	record.retainUntil = time.Now().Add(-time.Second)
	s.terminal[pendingRequest.ID] = record
	s.mu.Unlock()
	if w := statusCall(status, pendingRequest.ID, cookie); w.Code != http.StatusNotFound {
		t.Fatalf("status survived definitive cleanup: %d", w.Code)
	}
}

func TestPublicStatusIsGloballyRateLimited(t *testing.T) {
	s, h, events := startAuth(t)
	clientID := register(t, h)
	_, cookie, _ := requestConsent(t, h, clientID)
	request := <-events
	status := s.PublicStatusHandler()
	for i := 0; i < 120; i++ {
		if w := statusCall(status, request.ID, cookie); w.Code != http.StatusOK {
			t.Fatalf("legitimate polling %d: %d", i, w.Code)
		}
	}
	if w := statusCall(status, request.ID, cookie); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("rate-limit bypass: %d", w.Code)
	}
}
