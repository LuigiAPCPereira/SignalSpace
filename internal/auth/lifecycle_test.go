package auth

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLifecycleReconcilesLostDecisionAndCompletion(t *testing.T) {
	s, handler, events := startAuth(t)
	clientID := register(t, handler)
	_, cookie, csrf := requestConsent(t, handler, clientID)
	request := <-events
	// Simular resposta de decisão descartada: a segunda ação é GET, não POST.
	committed, err := s.DecideAndSnapshot(request.ID, 1, "approve")
	if err != nil || committed.Version != 2 || committed.DecidedAt == nil || committed.Status != "APPROVED" {
		t.Fatalf("decision did not record time and version: %+v %v", committed, err)
	}
	observed, err := s.GetRequestSnapshot(request.ID)
	if err != nil || observed.DecidedAt == nil || !observed.DecidedAt.Equal(*committed.DecidedAt) || observed.Version != 2 {
		t.Fatalf("lost decision response could not be reconciled: %+v %v", observed, err)
	}
	if _, err := s.DecideAndSnapshot(request.ID, 1, "deny"); !errors.Is(err, ErrOAuthAlreadyDecided) {
		t.Fatalf("replayed decision changed state: %v", err)
	}
	if result := complete(handler, request.ID, csrf, cookie); result.Code != http.StatusSeeOther {
		t.Fatalf("completion failed: %d", result.Code)
	}
	completed, err := s.GetRequestSnapshot(request.ID)
	if err != nil || completed.Status != "COMPLETED" || completed.Version != 3 || completed.DecidedAt == nil || !completed.DecidedAt.Equal(*committed.DecidedAt) {
		t.Fatalf("completed result not retained: %+v %v", completed, err)
	}
	encoded, err := json.Marshal(completed)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"csrf", "challenge", "session_hash", "access_token", "session_id"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("terminal model exposed %q", secret)
		}
	}
	if result := complete(handler, request.ID, csrf, cookie); result.Code != http.StatusForbidden {
		t.Fatalf("completion replay: %d", result.Code)
	}
	s.mu.Lock()
	codes := len(s.codes)
	s.mu.Unlock()
	if codes != 1 {
		t.Fatalf("completion issued %d codes", codes)
	}
}

func TestLifecycleDenialAndApprovedExpiryAreRetained(t *testing.T) {
	s, handler, events := startAuth(t)
	clientID := register(t, handler)
	_, cookie, csrf := requestConsent(t, handler, clientID)
	first := <-events
	if err := s.DecideTerminal(first.ID, false); err != nil {
		t.Fatal(err)
	}
	if result := complete(handler, first.ID, csrf, cookie); result.Code != http.StatusForbidden {
		t.Fatalf("denied request completed: %d", result.Code)
	}
	denied, err := s.GetRequestSnapshot(first.ID)
	if err != nil || denied.Status != "DENIED" || denied.Version != 2 || denied.DecidedAt == nil {
		t.Fatalf("denied tombstone unavailable: %+v %v", denied, err)
	}
	_, cookie, secondCSRF := requestConsent(t, handler, clientID)
	second := <-events
	if err := s.DecideTerminal(second.ID, true); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	p := s.pending[second.ID]
	p.Expires = time.Now().Add(-time.Second)
	s.pending[second.ID] = p
	s.mu.Unlock()
	expired, err := s.GetRequestSnapshot(second.ID)
	if err != nil || expired.Status != "EXPIRED" || expired.Version != 3 || expired.DecidedAt == nil {
		t.Fatalf("approved expiry lost version or decision: %+v %v", expired, err)
	}
	if err := s.DecideVersioned(second.ID, 2, "deny"); !errors.Is(err, ErrOAuthRequestExpired) {
		t.Fatalf("expired tombstone allowed decision: %v", err)
	}
	if result := complete(handler, second.ID, secondCSRF, cookie); result.Code != http.StatusForbidden {
		t.Fatalf("expired approved request reached completion: %d", result.Code)
	}
	s.mu.Lock()
	codes := len(s.codes)
	s.mu.Unlock()
	if codes != 0 {
		t.Fatalf("expired approved request issued %d authorization codes", codes)
	}
	if result := statusCall(s.PublicStatusHandler(), second.ID, cookie); result.Code != http.StatusOK || statusValue(t, result)["status"] != "EXPIRED" {
		t.Fatalf("expired public status not retained: %d", result.Code)
	}
}

func TestLifecycleTerminalCapAndTimeBound(t *testing.T) {
	s, _, _ := startAuth(t)
	now := time.Now()
	s.mu.Lock()
	for i := 0; i < maxTerminalRequests+16; i++ {
		id := fmt.Sprintf("%022d", i)
		p := pending{Scope: scope, Expires: now.Add(pendingTTL), CreatedAt: now, Approved: true, Version: 2, DecidedAt: now}
		s.terminalizeLocked(id, p, "COMPLETED", now.Add(time.Duration(i)*time.Millisecond))
	}
	if len(s.terminal) != maxTerminalRequests {
		s.mu.Unlock()
		t.Fatalf("terminal retention exceeded cap: %d", len(s.terminal))
	}
	_, oldest := s.terminal[fmt.Sprintf("%022d", 0)]
	_, newest := s.terminal[fmt.Sprintf("%022d", maxTerminalRequests+15)]
	s.pruneTerminalLocked(now.Add(11 * time.Minute))
	remaining := len(s.terminal)
	s.mu.Unlock()
	if oldest || !newest || remaining != 0 {
		t.Fatalf("terminal eviction incorrect: oldest=%t newest=%t remaining=%d", oldest, newest, remaining)
	}
}

func TestLifecycleRevokedGrantDoesNotConsumeApprovedRequest(t *testing.T) {
	allowed := true
	s, err := New(Config{ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope,
		ReadScope: "signalspace:workspace.read", CanIssueRead: func(string) bool { return allowed },
		StateDir: filepath.Join(t.TempDir(), "state"), OnRequest: func(RequestInfo) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := strings.Repeat("a", 22)
	csrf := "test-public-csrf"
	cookie := &http.Cookie{Name: "signalspace_auth", Value: "test-public-cookie"}
	now := time.Now()
	s.mu.Lock()
	s.pending[id] = pending{ClientID: "client-a", Redirect: callback, Scope: scope + " signalspace:workspace.read",
		Resource: resourceURL, State: "state-for-callback", CSRF: csrf, SessionHash: sha256.Sum256([]byte(cookie.Value)),
		CreatedAt: now, Expires: now.Add(pendingTTL), Version: 2, Approved: true, DecidedAt: now}
	s.mu.Unlock()
	form := url.Values{"request": {id}, "csrf": {csrf}}
	allowed = false
	first := invoke(s.Handler(), http.MethodPost, "/authorize/complete", form.Encode(), "application/x-www-form-urlencoded", cookie)
	if first.Code != http.StatusForbidden {
		t.Fatalf("revoked grant completed: %d", first.Code)
	}
	s.mu.Lock()
	_, exists := s.pending[id]
	codes := len(s.codes)
	s.mu.Unlock()
	if !exists || codes != 0 {
		t.Fatal("failed grant check consumed request or issued code")
	}
	allowed = true
	second := invoke(s.Handler(), http.MethodPost, "/authorize/complete", form.Encode(), "application/x-www-form-urlencoded", cookie)
	if second.Code != http.StatusSeeOther {
		t.Fatalf("valid completion after restored grant: %d", second.Code)
	}
	item, err := s.GetRequestSnapshot(id)
	if err != nil || item.Status != "COMPLETED" {
		t.Fatalf("restored grant did not complete: %+v %v", item, err)
	}
}

func TestLifecycleConcurrentCompletionIssuesOneCode(t *testing.T) {
	s, handler, events := startAuth(t)
	clientID := register(t, handler)
	_, cookie, csrf := requestConsent(t, handler, clientID)
	request := <-events
	if err := s.DecideTerminal(request.ID, true); err != nil {
		t.Fatal(err)
	}
	const workers = 20
	var group sync.WaitGroup
	start := make(chan struct{})
	results := make(chan int, workers)
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results <- complete(handler, request.ID, csrf, cookie).Code
		}()
	}
	close(start)
	group.Wait()
	close(results)
	success := 0
	for code := range results {
		if code == http.StatusSeeOther {
			success++
		} else if code != http.StatusForbidden {
			t.Fatalf("unexpected concurrent completion: %d", code)
		}
	}
	s.mu.Lock()
	issued := len(s.codes)
	s.mu.Unlock()
	if success != 1 || issued != 1 {
		t.Fatalf("concurrent completions succeeded=%d codes=%d", success, issued)
	}
}
