package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testClientA = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const testClientB = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"

func TestGrantsRequireOwnerAndExplicitGrant(t *testing.T) {
	if _, err := NewGrants(""); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("empty owner: %v", err)
	}
	g, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("approved"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := g.ReadText("local-owner", testClientA, "", "file"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("read without grant: %v", err)
	}
	for _, id := range []string{"", "Client with spaces", strings.Repeat("C", 33), strings.Repeat("!", 32)} {
		if _, err := g.Grant(root, id); !errors.Is(err, ErrNotAuthorized) {
			t.Fatalf("invalid OAuth client accepted: %q %v", id, err)
		}
	}
	if _, err := g.Grant("/", testClientA); !errors.Is(err, ErrInvalidRoot) {
		t.Fatalf("broad root: %v", err)
	}
	id, err := g.Grant(root, testClientA)
	if err != nil || id == "" {
		t.Fatalf("grant: %v", err)
	}
	for _, tc := range []struct{ owner, id string }{{"other-owner", id}, {"local-owner", "wrong"}} {
		if _, err := g.ReadText(tc.owner, testClientA, tc.id, "file"); !errors.Is(err, ErrNotAuthorized) {
			t.Fatalf("unauthorized read: %v", err)
		}
	}
	if _, err := g.ReadText("local-owner", testClientB, id, "file"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("other OAuth client gained access: %v", err)
	}
	if _, err := g.ReadText("local-owner", "", id, "file"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("missing client accepted: %v", err)
	}
	if content, err := g.ReadText("local-owner", testClientA, id, "file"); err != nil || content != "approved" {
		t.Fatalf("approved read: %q, %v", content, err)
	}
	if _, err := g.ReadText("local-owner", testClientA, id, "../file"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("traversal accepted: %v", err)
	}
	if err := g.Revoke("wrong"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("wrong revoke: %v", err)
	}
	if err := g.Revoke(id); err != nil {
		t.Fatal(err)
	}
	if _, err := g.ReadText("local-owner", testClientA, id, "file"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("read after revoke: %v", err)
	}
}

func TestGrantScopesKeepWriteIndependentFromRead(t *testing.T) {
	g, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}

	readOnly, err := g.Grant(root, testClientA)
	if err != nil {
		t.Fatal(err)
	}
	if !g.AllowsClient(testClientA) {
		t.Fatal("read scope was not retained")
	}
	if err := g.ReplaceText("local-owner", testClientA, readOnly, "file", "before", "after"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("read scope authorized write: %v", err)
	}

	writeOnly, err := g.GrantWithScopes(root, testClientA, ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	if g.AllowsClient(testClientA) {
		t.Fatal("write-only grant exposed read scope")
	}
	if err := g.ReplaceText("local-owner", testClientA, writeOnly, "file", "before", "after"); err != nil {
		t.Fatalf("write scope did not authorize edit: %v", err)
	}
	if _, err := g.ReadText("local-owner", testClientA, writeOnly, "file"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("write-only grant authorized read: %v", err)
	}

	if _, err := g.GrantWithScopes(root, testClientA, "signalspace:workspace.unknown"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("unknown scope accepted: %v", err)
	}
}

func TestGrantsReplacementAndShutdownRevoke(t *testing.T) {
	g, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	first := t.TempDir()
	second := t.TempDir()
	if err := os.WriteFile(filepath.Join(first, "file"), []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "file"), []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	oldID, err := g.Grant(first, testClientA)
	if err != nil {
		t.Fatal(err)
	}
	newID, err := g.Grant(second, testClientB)
	if err != nil || newID == oldID {
		t.Fatalf("replacement: %v", err)
	}
	if _, err := g.ReadText("local-owner", testClientA, oldID, "file"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("old grant active: %v", err)
	}
	if _, err := g.ReadText("local-owner", testClientA, newID, "file"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("previous client retained replacement grant: %v", err)
	}
	if content, err := g.ReadText("local-owner", testClientB, newID, "file"); err != nil || content != "second" {
		t.Fatalf("replacement content: %q, %v", content, err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := g.ReadText("local-owner", testClientB, newID, "file"); !errors.Is(err, ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
	if _, err := g.Grant(second, testClientB); !errors.Is(err, ErrClosed) {
		t.Fatalf("grant after close: %v", err)
	}
	if err := g.Revoke(newID); !errors.Is(err, ErrClosed) {
		t.Fatalf("revoke after close: %v", err)
	}
}

func TestGrantsSnapshotIsAbsentSafeOrderedAndIndependent(t *testing.T) {
	g, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	absent, err := g.Snapshot()
	if err != nil || absent.Active || absent.SessionID != "" || absent.ClientID != "" || len(absent.Scopes) != 0 {
		t.Fatalf("snapshot without grant: %+v, %v", absent, err)
	}

	root := t.TempDir()
	const privateContents = "private workspace contents"
	if err := os.WriteFile(filepath.Join(root, "file"), []byte(privateContents), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := g.GrantWithScopes(root, testClientA, ScopeTest, ScopeRead, ScopeGit, ScopeGitCommit, ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := g.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	wantScopes := []string{ScopeRead, ScopeWrite, ScopeGit, ScopeGitCommit, ScopeTest}
	if !snapshot.Active || snapshot.SessionID != id || snapshot.ClientID != testClientA || !reflect.DeepEqual(snapshot.Scopes, wantScopes) {
		t.Fatalf("active snapshot = %+v, want scopes %v", snapshot, wantScopes)
	}
	snapshot.Scopes[0] = "mutated by caller"
	copyAgain, err := g.Snapshot()
	if err != nil || !reflect.DeepEqual(copyAgain.Scopes, wantScopes) {
		t.Fatalf("caller changed stored scopes: %+v, %v", copyAgain, err)
	}

	// A forma serializada do snapshot não inclui raiz ou conteúdo de arquivos.
	serialized := fmt.Sprintf("%+v", copyAgain)
	if strings.Contains(serialized, root) || strings.Contains(serialized, privateContents) {
		t.Fatalf("snapshot exposed private workspace data: %s", serialized)
	}
	snapshotType := reflect.TypeOf(copyAgain)
	for _, forbidden := range []string{"Root", "FD", "Session"} {
		if _, ok := snapshotType.FieldByName(forbidden); ok {
			t.Fatalf("snapshot exposes forbidden field %q", forbidden)
		}
	}

	if err := g.Revoke(id); err != nil {
		t.Fatal(err)
	}
	revoked, err := g.Snapshot()
	if err != nil || revoked.Active || revoked.SessionID != "" || revoked.ClientID != "" || len(revoked.Scopes) != 0 {
		t.Fatalf("snapshot after revoke: %+v, %v", revoked, err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if snapshot, err := g.Snapshot(); !errors.Is(err, ErrClosed) || snapshot.Active {
		t.Fatalf("snapshot after close = %+v, %v", snapshot, err)
	}
}

func TestGrantsRevokeStaleSnapshotCannotRevokeReplacement(t *testing.T) {
	g, err := NewGrants("local-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	oldID, err := g.GrantWithScopes(t.TempDir(), testClientA, ScopeRead)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := g.Snapshot()
	if err != nil || !stale.Active || stale.SessionID != oldID {
		t.Fatalf("initial snapshot: %+v, %v", stale, err)
	}
	newID, err := g.GrantWithScopes(t.TempDir(), testClientB, ScopeWrite, ScopeTest)
	if err != nil {
		t.Fatal(err)
	}

	// Simula a substituição entre Snapshot e Revoke(current): o ID antigo é
	// revalidado atomicamente e não pode revogar a concessão recém-criada.
	if err := g.Revoke(stale.SessionID); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("stale revoke error = %v, want ErrNotAuthorized", err)
	}
	current, err := g.Snapshot()
	wantScopes := []string{ScopeWrite, ScopeTest}
	if err != nil || !current.Active || current.SessionID != newID || current.ClientID != testClientB || !reflect.DeepEqual(current.Scopes, wantScopes) {
		t.Fatalf("replacement grant was affected by stale revoke: %+v, %v", current, err)
	}
}
