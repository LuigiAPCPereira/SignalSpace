package auth

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestVersionedDecisionRejectsStaleDuplicateAndExpired(t *testing.T) {
	s, h, events := startAuth(t)
	clientID := register(t, h)
	_, _, _ = requestConsent(t, h, clientID)
	request := <-events
	if err := s.DecideVersioned(request.ID, 2, "approve"); !errors.Is(err, ErrOAuthStaleRequest) {
		t.Fatalf("stale version accepted: %v", err)
	}
	if err := s.DecideVersioned(request.ID, 1, "invalid"); !errors.Is(err, ErrOAuthInvalidDecision) {
		t.Fatalf("unknown decision accepted: %v", err)
	}
	if err := s.DecideVersioned(request.ID, 1, "approve"); err != nil {
		t.Fatal(err)
	}
	if err := s.DecideVersioned(request.ID, 1, "deny"); !errors.Is(err, ErrOAuthAlreadyDecided) {
		t.Fatalf("duplicate decision changed outcome: %v", err)
	}
	s.mu.Lock()
	item := s.pending[request.ID]
	if !item.Approved || item.Denied || len(s.codes) != 0 {
		s.mu.Unlock()
		t.Fatal("decision emitted code or lost approved status")
	}
	item.Expires = time.Now().Add(-time.Second)
	s.pending[request.ID] = item
	s.mu.Unlock()
	if err := s.DecideVersioned(request.ID, 1, "deny"); !errors.Is(err, ErrOAuthRequestExpired) {
		t.Fatalf("expired request accepted: %v", err)
	}
	if err := s.DecideVersioned(strings.Repeat("z", 22), 1, "approve"); !errors.Is(err, ErrOAuthRequestNotFound) {
		t.Fatalf("unknown request accepted: %v", err)
	}
}

func TestVersionedDecisionRequiresLiveReadGrantButAllowsDenial(t *testing.T) {
	allowed := false
	s, err := New(Config{
		ResourceURL:  resourceURL,
		Issuer:       "https://signalspace.example",
		Scope:        scope,
		ReadScope:    "signalspace:workspace.read",
		CanIssueRead: func(clientID string) bool { return allowed && clientID == "client-a" },
		StateDir:     filepath.Join(t.TempDir(), "state"),
		OnRequest:    func(RequestInfo) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	firstID := strings.Repeat("a", 22)
	secondID := strings.Repeat("b", 22)
	entry := pending{ClientID: "client-a", Scope: scope + " signalspace:workspace.read", Expires: time.Now().Add(time.Minute)}
	s.pending[firstID] = entry
	s.pending[secondID] = entry
	if err := s.DecideVersioned(firstID, 1, "approve"); !errors.Is(err, ErrOAuthGrantRequired) {
		t.Fatalf("revoked workspace permitted approval: %v", err)
	}
	if s.pending[firstID].Approved || s.pending[firstID].Denied {
		t.Fatal("revoked grant mutated request")
	}
	if err := s.DecideVersioned(firstID, 1, "deny"); err != nil {
		t.Fatalf("denial after grant revocation failed: %v", err)
	}
	allowed = true
	if err := s.DecideVersioned(secondID, 1, "approve"); err != nil {
		t.Fatalf("active grant could not be approved: %v", err)
	}
	if !s.pending[secondID].Approved || len(s.codes) != 0 {
		t.Fatal("approval issued code or was not recorded")
	}
}

func TestVersionedDecisionAndTerminalRaceCommitOnce(t *testing.T) {
	s, h, events := startAuth(t)
	clientID := register(t, h)
	_, _, _ = requestConsent(t, h, clientID)
	request := <-events
	const workers = 24
	var group sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, workers+1)
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results <- s.DecideVersioned(request.ID, 1, "approve")
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		<-start
		results <- s.Approve(request.ID, false)
	}()
	close(start)
	group.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent decisions committed %d times", success)
	}
	s.mu.Lock()
	item := s.pending[request.ID]
	s.mu.Unlock()
	if item.Approved == item.Denied {
		t.Fatal("request has conflicting or missing terminal state")
	}
}
