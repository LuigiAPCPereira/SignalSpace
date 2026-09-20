package auth

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestDeniedRequestRemainsDeniedWhenDeadlinePasses(t *testing.T) {
	s, handler, events := startAuth(t)
	clientID := register(t, handler)
	_, cookie, _ := requestConsent(t, handler, clientID)
	request := <-events
	decided, err := s.DecideAndSnapshot(request.ID, 1, "deny")
	if err != nil || decided.DecidedAt == nil {
		t.Fatalf("missing denied decision: %+v %v", decided, err)
	}
	s.mu.Lock()
	p := s.pending[request.ID]
	p.Expires = time.Now().Add(-time.Second)
	s.pending[request.ID] = p
	s.mu.Unlock()
	if err := s.DecideVersioned(request.ID, 1, "approve"); !errors.Is(err, ErrOAuthAlreadyDecided) {
		t.Fatalf("denial was lost at deadline: %v", err)
	}
	item, err := s.GetRequestSnapshot(request.ID)
	if err != nil || item.Status != "DENIED" || item.Version != 2 || item.DecidedAt == nil || !item.DecidedAt.Equal(*decided.DecidedAt) {
		t.Fatalf("denial changed on expiration: %+v %v", item, err)
	}
	response := statusCall(s.PublicStatusHandler(), request.ID, cookie)
	if response.Code != http.StatusOK || statusValue(t, response)["status"] != "DENIED" {
		t.Fatalf("public denial changed: %d %s", response.Code, response.Body.String())
	}
	if len(s.codes) != 0 {
		t.Fatal("denied request issued code")
	}
}
