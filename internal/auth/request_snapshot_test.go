package auth

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequestSnapshotsUseRealOAuthStateWithoutSecrets(t *testing.T) {
	s, handler, events := startAuth(t)
	clientID := register(t, handler)
	_, _, _ = requestConsent(t, handler, clientID)
	request := <-events
	list := s.ListRequestSnapshots()
	if len(list) != 1 || list[0].ID != request.ID || list[0].Status != "PENDING" || list[0].Version != 1 {
		t.Fatalf("pending request unavailable: %+v", list)
	}
	item, err := s.GetRequestSnapshot(request.ID)
	if err != nil || item.Client.ID != clientID || item.Client.Verified || item.Client.DisplayName != "ChatGPT" || item.WorkspaceRead.Required || item.WorkspaceRead.GrantStatus != "NOT_APPLICABLE" {
		t.Fatalf("unsafe or incorrect request: %+v %v", item, err)
	}
	if item.CreatedAt.After(item.ExpiresAt) || item.ExpiresAt.Sub(item.CreatedAt) != pendingTTL || item.RedirectURI != callback || item.Scope != scope {
		t.Fatalf("OAuth snapshot does not match the validated request: %+v", item)
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{"session_hash", "code_challenge", "csrf", "access_token", "session_id", "root_path"} {
		if strings.Contains(string(encoded), sensitive) {
			t.Fatalf("snapshot exposes private field %q", sensitive)
		}
	}
	decided, err := s.DecideAndSnapshot(request.ID, 1, "approve")
	if err != nil || decided.Status != "APPROVED" || decided.Version != 2 || len(s.codes) != 0 {
		t.Fatalf("decision snapshot or code emission: %+v %v", decided, err)
	}
	if _, err := s.DecideAndSnapshot(request.ID, 1, "deny"); !errors.Is(err, ErrOAuthAlreadyDecided) {
		t.Fatalf("duplicate decision accepted: %v", err)
	}
	if _, err := s.GetRequestSnapshot(strings.Repeat("z", 22)); !errors.Is(err, ErrOAuthRequestNotFound) {
		t.Fatalf("unknown request returned data: %v", err)
	}
}

func TestRequestSnapshotsRecheckReadGrantAndExpiration(t *testing.T) {
	allowed := false
	s, err := New(Config{
		ResourceURL:  resourceURL,
		Issuer:       "https://signalspace.example",
		Scope:        scope,
		ReadScope:    "signalspace:workspace.read",
		CanIssueRead: func(clientID string) bool { return allowed && clientID == "client-a" },
		StateDir:     filepath.Join(t.TempDir(), "identity"),
		OnRequest:    func(RequestInfo) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := strings.Repeat("a", 22)
	s.pending[id] = pending{ClientID: "client-a", Scope: scope + " signalspace:workspace.read", Expires: time.Now().Add(time.Minute)}
	item, err := s.GetRequestSnapshot(id)
	if err != nil || item.WorkspaceRead.GrantStatus != "REVOKED" || !item.WorkspaceRead.Required {
		t.Fatalf("revoked grant not visible: %+v %v", item, err)
	}
	allowed = true
	item, err = s.GetRequestSnapshot(id)
	if err != nil || item.WorkspaceRead.GrantStatus != "ACTIVE" {
		t.Fatalf("live grant was not rechecked: %+v %v", item, err)
	}
	s.mu.Lock()
	pendingItem := s.pending[id]
	pendingItem.Expires = time.Now().Add(-time.Second)
	s.pending[id] = pendingItem
	s.mu.Unlock()
	item, err = s.GetRequestSnapshot(id)
	if err != nil || item.Status != "EXPIRED" || item.Version != 2 {
		t.Fatalf("expired request not observable: %+v %v", item, err)
	}
	if _, err := s.DecideAndSnapshot(id, 1, "approve"); !errors.Is(err, ErrOAuthRequestExpired) {
		t.Fatalf("expired request approved: %v", err)
	}
}
