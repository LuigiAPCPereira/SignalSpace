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

func TestDirectoryListingOnlyNamesAndRepeatableRoot(t *testing.T) {
	s, root := approvedSession(t)
	if names, err := s.ListDirectory("."); err != nil || len(names) != 0 {
		t.Fatalf("empty approved directory: %v %v", names, err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "z.txt"), []byte("PRIVATE_CONTENT"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden"), []byte("hidden"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "inside.txt"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	want := []string{".hidden", "link", "nested", "z.txt"}
	for i := 0; i < 2; i++ {
		names, err := s.ListDirectory(".")
		if err != nil || !reflect.DeepEqual(names, want) {
			t.Fatalf("root listing %d: %v %v", i, names, err)
		}
		if strings.Contains(strings.Join(names, " "), "PRIVATE_CONTENT") {
			t.Fatal("listing returned file content")
		}
	}
	if names, err := s.ListDirectory("nested"); err != nil || !reflect.DeepEqual(names, []string{"inside.txt"}) {
		t.Fatalf("nested listing: %v %v", names, err)
	}
	for _, path := range []string{"", "../", "nested/..", "nested//", "/etc", "./nested", "nested\\other", "bad\x00name", strings.Repeat("x", 4097)} {
		if names, err := s.ListDirectory(path); !errors.Is(err, ErrInvalidPath) || names != nil {
			t.Fatalf("invalid path %q: %v %v", path, names, err)
		}
	}
	if names, err := s.ListDirectory("link"); !errors.Is(err, ErrUnsafePath) || names != nil {
		t.Fatalf("symlink directory: %v %v", names, err)
	}
	if names, err := s.ListDirectory("z.txt"); !errors.Is(err, ErrUnsafePath) || names != nil {
		t.Fatalf("regular file as directory: %v %v", names, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if names, err := s.ListDirectory("."); !errors.Is(err, ErrClosed) || names != nil {
		t.Fatalf("listing after close: %v %v", names, err)
	}
}

func TestDirectoryListingBoundedWithoutPartialResults(t *testing.T) {
	s, root := approvedSession(t)
	for i := 0; i < MaxDirectoryEntries; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("file-%03d", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if names, err := s.ListDirectory("."); err != nil || len(names) != MaxDirectoryEntries {
		t.Fatalf("exact limit: %d %v", len(names), err)
	}
	if err := os.WriteFile(filepath.Join(root, "overflow"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if names, err := s.ListDirectory("."); !errors.Is(err, ErrTooManyEntries) || names != nil {
		t.Fatalf("overflow disclosed partial listing: %v %v", names, err)
	}
}

func TestDirectoryListingRejectsInvalidUTF8Names(t *testing.T) {
	s, root := approvedSession(t)
	if err := os.WriteFile(filepath.Join(root, string([]byte{0xff})), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if names, err := s.ListDirectory("."); !errors.Is(err, ErrInvalidPath) || names != nil {
		t.Fatalf("invalid filename silently rewritten: %v %v", names, err)
	}
}

func TestDirectoryListingCannotEscapeReplacedRoot(t *testing.T) {
	s, root := approvedSession(t)
	if err := os.WriteFile(filepath.Join(root, "approved"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "OUTSIDE_MARKER"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	moved := root + "-moved"
	t.Cleanup(func() { _ = os.RemoveAll(moved) })
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	if names, err := s.ListDirectory("."); err != nil || !reflect.DeepEqual(names, []string{"approved"}) {
		t.Fatalf("root path replacement redirected listing: %v %v", names, err)
	}
}

func TestDirectoryListingRequiresCurrentGrant(t *testing.T) {
	g, err := NewGrants("owner")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "approved"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if names, err := g.ListDirectory("owner", testClientA, "missing", "."); !errors.Is(err, ErrNotAuthorized) || names != nil {
		t.Fatalf("listing without grant: %v %v", names, err)
	}
	first, err := g.Grant(root, testClientA)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ owner, client, id string }{
		{"other", testClientA, first},
		{"owner", testClientB, first},
		{"owner", "", first},
		{"owner", testClientA, "wrong"},
	} {
		if names, err := g.ListDirectory(tc.owner, tc.client, tc.id, "."); !errors.Is(err, ErrNotAuthorized) || names != nil {
			t.Fatalf("unauthorized listing: %v %v", names, err)
		}
	}
	if names, err := g.ListDirectory("owner", testClientA, first, "."); err != nil || !reflect.DeepEqual(names, []string{"approved"}) {
		t.Fatalf("authorized listing: %v %v", names, err)
	}
	second, err := g.Grant(root, testClientB)
	if err != nil || second == first {
		t.Fatalf("replacement: %v", err)
	}
	if names, err := g.ListDirectory("owner", testClientA, first, "."); !errors.Is(err, ErrNotAuthorized) || names != nil {
		t.Fatalf("old grant retained listing: %v %v", names, err)
	}
	if names, err := g.ListDirectory("owner", testClientB, second, "."); err != nil || !reflect.DeepEqual(names, []string{"approved"}) {
		t.Fatalf("new grant denied: %v %v", names, err)
	}
	if err := g.Revoke(second); err != nil {
		t.Fatal(err)
	}
	if names, err := g.ListDirectory("owner", testClientB, second, "."); !errors.Is(err, ErrNotAuthorized) || names != nil {
		t.Fatalf("revoked grant still lists: %v %v", names, err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if names, err := g.ListDirectory("owner", testClientB, second, "."); !errors.Is(err, ErrClosed) || names != nil {
		t.Fatalf("listing after shutdown: %v %v", names, err)
	}
}
