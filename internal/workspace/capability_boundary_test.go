package workspace

import (
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
)

func TestGrantStoresCapabilitiesAndKeepsLegacyScopeSnapshot(t *testing.T) {
	grants, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()

	root := t.TempDir()
	if _, err := grants.GrantWithScopes(root, testClientA, ScopeRead, ScopeGit); err != nil {
		t.Fatal(err)
	}
	if _, ok := grants.capabilities[capability.WorkspaceRead]; !ok {
		t.Fatal("legacy read scope was not adapted to workspace.read")
	}
	if _, ok := grants.capabilities[capability.GitReview]; !ok {
		t.Fatal("legacy Git scope was not adapted to git.review")
	}
	if _, ok := grants.capabilities[capability.Capability(ScopeRead)]; ok {
		t.Fatal("raw OAuth scope was stored as an internal capability")
	}
	snapshot, err := grants.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ScopeRead, ScopeGit}
	if len(snapshot.Scopes) != len(want) || snapshot.Scopes[0] != want[0] || snapshot.Scopes[1] != want[1] {
		t.Fatalf("legacy snapshot = %v, want %v", snapshot.Scopes, want)
	}
}
