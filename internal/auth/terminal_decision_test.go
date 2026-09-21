package auth

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTerminalDecisionRejectsRevokedReadGrantAndAllowsDenial(t *testing.T) {
	allowed := false
	s, err := New(Config{
		ResourceURL: resourceURL,
		Issuer:      "https://signalspace.example",
		Scope:       scope,
		ReadScope:   "signalspace:workspace.read",
		CanIssueRead: func(clientID string) bool {
			return allowed && clientID == "client-a"
		},
		StateDir:  filepath.Join(t.TempDir(), "state"),
		OnRequest: func(RequestInfo) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := strings.Repeat("t", 22)
	s.pending[id] = pending{ClientID: "client-a", Scope: scope + " signalspace:workspace.read", Expires: time.Now().Add(time.Minute)}

	if err := s.DecideTerminal(id, true); !errors.Is(err, ErrOAuthGrantRequired) {
		t.Fatalf("terminal bypassed revoked workspace grant: %v", err)
	}
	if s.pending[id].Approved || s.pending[id].Denied || len(s.codes) != 0 {
		t.Fatal("failed terminal approval changed OAuth state")
	}
	if err := s.DecideTerminal(id, false); err != nil {
		t.Fatalf("denial after grant revocation failed: %v", err)
	}
	if err := s.DecideTerminal(id, true); !errors.Is(err, ErrOAuthAlreadyDecided) {
		t.Fatalf("terminal overwrote the recorded denial: %v", err)
	}
	if !s.pending[id].Denied || s.pending[id].Approved || len(s.codes) != 0 {
		t.Fatal("terminal denial changed or issued an OAuth code")
	}
}

func TestTerminalDecisionAndPanelShareSingleCommit(t *testing.T) {
	s, h, events := startAuth(t)
	clientID := register(t, h)
	_, _, _ = requestConsent(t, h, clientID)
	request := <-events

	const workers = 20
	var group sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, workers+1)
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := s.DecideAndSnapshot(request.ID, 1, "approve")
			results <- err
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		<-start
		results <- s.DecideTerminal(request.ID, false)
	}()
	close(start)
	group.Wait()
	close(results)

	committed := 0
	for err := range results {
		if err == nil {
			committed++
		} else if !errors.Is(err, ErrOAuthAlreadyDecided) {
			t.Fatalf("unexpected concurrent decision error: %v", err)
		}
	}
	if committed != 1 {
		t.Fatalf("terminal/panel committed %d decisions, expected one", committed)
	}
	s.mu.Lock()
	item := s.pending[request.ID]
	codes := len(s.codes)
	s.mu.Unlock()
	if item.Approved == item.Denied || codes != 0 {
		t.Fatal("conflicting decision or code issued before public completion")
	}
}

func TestTerminalDecisionRejectsExpiredRequest(t *testing.T) {
	s, h, events := startAuth(t)
	clientID := register(t, h)
	_, _, _ = requestConsent(t, h, clientID)
	request := <-events
	s.mu.Lock()
	item := s.pending[request.ID]
	item.Expires = time.Now().Add(-time.Second)
	s.pending[request.ID] = item
	s.mu.Unlock()
	if err := s.DecideTerminal(request.ID, true); !errors.Is(err, ErrOAuthRequestExpired) {
		t.Fatalf("terminal approved expired request: %v", err)
	}
}
