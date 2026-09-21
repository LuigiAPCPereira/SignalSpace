package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestReplaceTextRequiresExactExpectedContentAndPreservesFileMode(t *testing.T) {
	s, root := approvedSession(t)
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceText("notes.txt", "wrong\n", "after\n"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale expected content: %v", err)
	}
	got, err := s.ReadText("notes.txt")
	if err != nil || got != "before\n" {
		t.Fatalf("conflict changed content: %q, %v", got, err)
	}
	if err := s.ReplaceText("notes.txt", "before\n", "after\n"); err != nil {
		t.Fatal(err)
	}
	got, err = s.ReadText("notes.txt")
	if err != nil || got != "after\n" {
		t.Fatalf("replacement: %q, %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("replacement changed permissions: %o", info.Mode().Perm())
	}
}

func TestReplaceTextAppliesTheSamePathAndContentBoundariesAsRead(t *testing.T) {
	s, root := approvedSession(t)
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "file.txt"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"", ".", "../secret", "nested/../file.txt", "/etc/passwd", "nested\\file.txt", "bad\x00name"} {
		if err := s.ReplaceText(path, "", "new"); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("unsafe path %q: %v", path, err)
		}
	}
	if err := s.ReplaceText("link", "outside", "new"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink edit escaped root: %v", err)
	}
	if err := s.ReplaceText("nested/file.txt", "ok", strings.Repeat("x", MaxTextBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized replacement: %v", err)
	}
	if err := s.ReplaceText("nested/file.txt", "ok\x00", "new"); !errors.Is(err, ErrNotText) {
		t.Fatalf("invalid expected text: %v", err)
	}
	if err := s.ReplaceText("nested/file.txt", "ok", "new"); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceTextIsDeniedAfterSessionClose(t *testing.T) {
	s, root := approvedSession(t)
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceText("file.txt", "before", "after"); !errors.Is(err, ErrClosed) {
		t.Fatalf("edit after close: %v", err)
	}
}

func TestGrantReplaceTextRequiresCurrentIdentityAndRevocation(t *testing.T) {
	grants, err := NewGrants("owner")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grants.Close() })
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := grants.Grant(root, testClientA)
	if err != nil {
		t.Fatal(err)
	}
	if err := grants.ReplaceText("other-owner", testClientA, id, "file.txt", "before", "after"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("other owner edited: %v", err)
	}
	if err := grants.ReplaceText("owner", testClientB, id, "file.txt", "before", "after"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("other client edited: %v", err)
	}
	if err := grants.ReplaceText("owner", testClientA, id, "file.txt", "before", "after"); err != nil {
		t.Fatal(err)
	}
	if err := grants.Revoke(id); err != nil {
		t.Fatal(err)
	}
	if err := grants.ReplaceText("owner", testClientA, id, "file.txt", "after", "denied"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("edit after revoke: %v", err)
	}
}

func TestConcurrentReplaceTextAllowsOnlyOneMatchingExpectedVersion(t *testing.T) {
	grants, err := NewGrants("owner")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grants.Close() })
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := grants.Grant(root, testClientA)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- grants.ReplaceText("owner", testClientA, id, "file.txt", "before", "after")
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var successes, conflicts int
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent edit result: %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}
