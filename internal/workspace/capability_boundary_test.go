package workspace

import (
	"errors"
	"os"
	"path/filepath"
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
	wantCapabilities := []capability.Capability{capability.WorkspaceRead, capability.GitReview}
	if len(snapshot.Capabilities) != len(wantCapabilities) || snapshot.Capabilities[0] != wantCapabilities[0] || snapshot.Capabilities[1] != wantCapabilities[1] {
		t.Fatalf("capability snapshot = %v, want %v", snapshot.Capabilities, wantCapabilities)
	}
	want := []string{ScopeRead, ScopeGit}
	if len(snapshot.Scopes) != len(want) || snapshot.Scopes[0] != want[0] || snapshot.Scopes[1] != want[1] {
		t.Fatalf("legacy snapshot = %v, want %v", snapshot.Scopes, want)
	}
}

func TestLegacyWriteMapsDeleteButTypedWriteDoesNot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "delete.txt"), []byte("delete"), 0600); err != nil {
		t.Fatal(err)
	}
	typed, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	typedID, err := typed.GrantWithCapabilities(root, testClientA, capability.WorkspaceWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := typed.DeleteFile("local-owner", testClientA, typedID, "delete.txt"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("typed workspace.write authorized delete: %v", err)
	}
	if err := typed.Close(); err != nil {
		t.Fatal(err)
	}

	legacy, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	legacyID, err := legacy.GrantWithScopes(root, testClientA, ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.AllowsClientCapability(testClientA, capability.WorkspaceDelete) {
		t.Fatal("legacy write did not apply compatibility delete mapping")
	}
	if _, err := legacy.DeleteFile("local-owner", testClientA, legacyID, "delete.txt"); err != nil {
		t.Fatalf("legacy write did not preserve delete behavior: %v", err)
	}
}
