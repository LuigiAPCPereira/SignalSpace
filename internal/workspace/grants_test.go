package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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
	if _, err := g.ReadText("local-owner", "", "file"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("read without grant: %v", err)
	}
	if _, err := g.Grant("/"); !errors.Is(err, ErrInvalidRoot) {
		t.Fatalf("broad root: %v", err)
	}
	id, err := g.Grant(root)
	if err != nil || id == "" {
		t.Fatalf("grant: %v", err)
	}
	for _, tc := range []struct{ owner, id string }{{"other-owner", id}, {"local-owner", "wrong"}} {
		if _, err := g.ReadText(tc.owner, tc.id, "file"); !errors.Is(err, ErrNotAuthorized) {
			t.Fatalf("unauthorized read: %v", err)
		}
	}
	if content, err := g.ReadText("local-owner", id, "file"); err != nil || content != "approved" {
		t.Fatalf("approved read: %q, %v", content, err)
	}
	if _, err := g.ReadText("local-owner", id, "../file"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("traversal accepted: %v", err)
	}
	if err := g.Revoke("wrong"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("wrong revoke: %v", err)
	}
	if err := g.Revoke(id); err != nil {
		t.Fatal(err)
	}
	if _, err := g.ReadText("local-owner", id, "file"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("read after revoke: %v", err)
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
	oldID, err := g.Grant(first)
	if err != nil {
		t.Fatal(err)
	}
	newID, err := g.Grant(second)
	if err != nil || newID == oldID {
		t.Fatalf("replacement: %v", err)
	}
	if _, err := g.ReadText("local-owner", oldID, "file"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("old grant active: %v", err)
	}
	if content, err := g.ReadText("local-owner", newID, "file"); err != nil || content != "second" {
		t.Fatalf("replacement content: %q, %v", content, err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := g.ReadText("local-owner", newID, "file"); !errors.Is(err, ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
	if _, err := g.Grant(second); !errors.Is(err, ErrClosed) {
		t.Fatalf("grant after close: %v", err)
	}
	if err := g.Revoke(newID); !errors.Is(err, ErrClosed) {
		t.Fatalf("revoke after close: %v", err)
	}
}
