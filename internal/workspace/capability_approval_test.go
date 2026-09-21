package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const approvalTestClient = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestCapabilityApprovalRequiresExplicitConfirmationAndKeepsScopesIndependent(t *testing.T) {
	grants, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	eligible := true
	approval, err := NewCapabilityApproval(grants, func(clientID string) bool {
		return eligible && clientID == approvalTestClient
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	request, err := approval.Request(root, approvalTestClient, ScopeTest, ScopeWrite, ScopeGit)
	if err != nil {
		t.Fatal(err)
	}
	if request.ID == "" || request.Root != root || request.ClientID != approvalTestClient ||
		len(request.Scopes) != 3 || request.Scopes[0] != ScopeWrite || request.Scopes[1] != ScopeGit || request.Scopes[2] != ScopeTest {
		t.Fatalf("unexpected pending request: %+v", request)
	}
	if grants.AllowsClientScope(approvalTestClient, ScopeWrite) || grants.AllowsClientScope(approvalTestClient, ScopeTest) || grants.AllowsClientScope(approvalTestClient, ScopeGit) {
		t.Fatal("request created a grant before confirmation")
	}
	if _, _, err := approval.Confirm("wrong"); !errors.Is(err, ErrCapabilityApprovalMissing) {
		t.Fatalf("wrong confirmation result: %v", err)
	}
	confirmed, sessionID, err := approval.Confirm(request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.ID != request.ID || sessionID == "" || sessionID == request.ID {
		t.Fatalf("confirmation did not return the local session: request=%+v session=%q", confirmed, sessionID)
	}
	// A confirmation creates exactly the selected capabilities, without read.
	if !grants.AllowsClientScope(approvalTestClient, ScopeWrite) || !grants.AllowsClientScope(approvalTestClient, ScopeTest) || !grants.AllowsClientScope(approvalTestClient, ScopeGit) || grants.AllowsClientScope(approvalTestClient, ScopeRead) {
		t.Fatal("confirmed capabilities were not independent")
	}
	if err := grants.Revoke(sessionID); err != nil {
		t.Fatalf("revoke after confirmation: %v", err)
	}
	if grants.AllowsClientScope(approvalTestClient, ScopeWrite) || grants.AllowsClientScope(approvalTestClient, ScopeTest) || grants.AllowsClientScope(approvalTestClient, ScopeGit) {
		t.Fatal("revoke retained a confirmed capability")
	}
}

func TestCapabilityApprovalCancelExpirationAndValidation(t *testing.T) {
	grants, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	eligible := true
	approval, err := NewCapabilityApproval(grants, func(string) bool { return eligible })
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, scopes := range [][]string{
		{"signalspace:unknown"},
		{ScopeWrite, ScopeWrite},
		{ScopeWrite, " " + ScopeTest},
	} {
		if _, err := approval.Request(root, approvalTestClient, scopes...); !errors.Is(err, ErrInvalidCapabilities) {
			t.Fatalf("invalid capability selection accepted %v: %v", scopes, err)
		}
	}
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := approval.Request(link, approvalTestClient, ScopeRead); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink root accepted: %v", err)
	}
	request, err := approval.Request(root, approvalTestClient, ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := approval.Request(root, approvalTestClient, ScopeWrite); !errors.Is(err, ErrCapabilityApprovalPending) {
		t.Fatalf("second pending request accepted: %v", err)
	}
	if err := approval.Cancel(request.ID); err != nil {
		t.Fatal(err)
	}
	if err := approval.Cancel(request.ID); !errors.Is(err, ErrCapabilityApprovalMissing) {
		t.Fatalf("canceled request remained usable: %v", err)
	}

	request, err = approval.Request(root, approvalTestClient, ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	approval.now = func() time.Time { return request.Expires.Add(time.Second) }
	if _, _, err := approval.Confirm(request.ID); !errors.Is(err, ErrCapabilityApprovalMissing) {
		t.Fatalf("expired request was confirmed: %v", err)
	}
	approval.now = time.Now
	request, err = approval.Request(root, approvalTestClient, ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	eligible = false
	if _, _, err := approval.Confirm(request.ID); !errors.Is(err, ErrCapabilityClientRejected) {
		t.Fatalf("client became ineligible but was confirmed: %v", err)
	}
	if grants.AllowsClientScope(approvalTestClient, ScopeWrite) {
		t.Fatal("ineligible client received a grant")
	}
}

func TestCapabilityApprovalCloseDiscardsPendingRequests(t *testing.T) {
	grants, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	approval, err := NewCapabilityApproval(grants, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	request, err := approval.Request(t.TempDir(), approvalTestClient, ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	approval.Close()
	if _, _, err := approval.Confirm(request.ID); !errors.Is(err, ErrCapabilityApprovalMissing) {
		t.Fatalf("closed approval retained request: %v", err)
	}
	if _, err := approval.Request(t.TempDir(), approvalTestClient, ScopeRead); !errors.Is(err, ErrCapabilityApprovalUnavailable) {
		t.Fatalf("closed approval accepted new request: %v", err)
	}
}
