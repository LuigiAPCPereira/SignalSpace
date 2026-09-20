package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func approvedSession(t *testing.T) (*Session, string) {
	t.Helper()
	root := t.TempDir()
	s, err := OpenApprovedRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, root
}

func TestApprovedRootReadsTextAndHasSessionIdentity(t *testing.T) {
	s, root := approvedSession(t)
	id := s.ID()
	if len(id) != 32 || id != s.ID() {
		t.Fatal("invalid stable session identifier")
	}
	if err := os.Mkdir(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadText("src/main.go")
	if err != nil || got != "package main\n" {
		t.Fatalf("read got %q: %v", got, err)
	}
	second, err := OpenApprovedRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.ID() == s.ID() {
		t.Fatal("separate sessions reused an identifier")
	}
}

func TestRootMustBeExplicitAndCannotBeHomeOrSymlink(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", ".", "project", "/", filepath.Clean(home)} {
		if session, err := OpenApprovedRoot(path); !errors.Is(err, ErrInvalidRoot) {
			if session != nil {
				_ = session.Close()
			}
			t.Fatalf("accepted broad or implicit root %q: %v", path, err)
		}
	}
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "approved")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if session, err := OpenApprovedRoot(link); !errors.Is(err, ErrUnsafePath) {
		if session != nil {
			_ = session.Close()
		}
		t.Fatalf("accepted symlink root: %v", err)
	}
}

func TestRejectsTraversalAbsoluteAndNoncanonicalPaths(t *testing.T) {
	s, _ := approvedSession(t)
	for _, path := range []string{"", ".", "../secret", "dir/../secret", "/etc/passwd", "./file", "dir//file", "dir/./file", "dir\\file", "bad\x00name", strings.Repeat("x", 4097)} {
		if _, err := s.ReadText(path); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("unsafe path %q: %v", path, err)
		}
	}
}

func TestRejectsSymlinkInsideRootAndEscapeOutside(t *testing.T) {
	s, root := approvedSession(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "file-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "dir-link")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"file-link", "dir-link/secret"} {
		if text, err := s.ReadText(path); !errors.Is(err, ErrUnsafePath) || text != "" {
			t.Fatalf("symlink escape %q returned %q, %v", path, text, err)
		}
	}
}

func TestRejectsOversizeBinaryAndDirectories(t *testing.T) {
	s, root := approvedSession(t)
	if err := os.WriteFile(filepath.Join(root, "large"), []byte(strings.Repeat("a", MaxTextBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "binary"), []byte{0xff, 0x00}, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		want error
	}{
		{"large", ErrTooLarge}, {"binary", ErrNotText}, {".", ErrInvalidPath},
	} {
		if text, err := s.ReadText(tc.path); !errors.Is(err, tc.want) || text != "" {
			t.Fatalf("path %q yielded %q, %v", tc.path, text, err)
		}
	}
	if _, err := s.ReadText("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := s.ReadText("subdir"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadText("subdir"); !errors.Is(err, ErrNotFile) {
		t.Fatalf("directory read: %v", err)
	}
}

func TestCloseRevokesSession(t *testing.T) {
	s, root := approvedSession(t)
	if err := os.WriteFile(filepath.Join(root, "f"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadText("f"); !errors.Is(err, ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
}

func TestDescriptorStaysOnApprovedDirectoryAfterPathReplacement(t *testing.T) {
	s, root := approvedSession(t)
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("approved"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "file"), []byte("outside"), 0600); err != nil {
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
	got, err := s.ReadText("file")
	if err != nil || got != "approved" {
		t.Fatalf("root path replacement redirected an existing session: %q, %v", got, err)
	}
}
