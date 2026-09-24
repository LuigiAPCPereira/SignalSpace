package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTypedFilesystemReadOperationsAreBoundedAndDoNotFollowSymlinks(t *testing.T) {
	session, root := approvedSession(t)
	if err := os.Mkdir(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\nneedle here\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("needle outside code\n"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("needle secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path string
		kind FileKind
		size int64
	}{
		{".", FileKindDirectory, 0},
		{"src/main.go", FileKindRegular, int64(len("package main\nneedle here\n"))},
		{"link", FileKindSymlink, 0},
		{"missing.txt", FileKindAbsent, 0},
	} {
		got, err := session.StatPath(tc.path)
		if err != nil || got.Kind != tc.kind || got.Found != (tc.kind != FileKindAbsent) || got.Size != tc.size {
			t.Fatalf("stat %q: %#v, %v", tc.path, got, err)
		}
	}

	found, err := session.FindPaths(".", "*.go", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(found.Matches, []FindMatch{{Path: "src/main.go", Kind: FileKindRegular}}) || found.Truncated {
		t.Fatalf("find result: %#v", found)
	}
	search, err := session.SearchText(".", "needle", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Matches) != 2 || search.Matches[0].Path != "notes.txt" || search.Matches[1].Path != "src/main.go" || search.Truncated || search.FilesSkipped != 0 {
		t.Fatalf("search result: %#v", search)
	}
	if _, err := session.FindPaths("link", "*", 0, 0); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("find followed symlink root: %v", err)
	}
	if _, err := session.SearchText("../", "needle", 0); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("search accepted traversal: %v", err)
	}
}

func TestTypedFilesystemWritesAreCreateOnlyAndHashPreconditioned(t *testing.T) {
	session, root := approvedSession(t)
	if result, err := session.CreateDirectory("created"); err != nil || result.Status != "created" {
		t.Fatalf("create directory: %#v, %v", result, err)
	}
	if result, err := session.CreateDirectory("created"); err != nil || result.Status != "already_exists" {
		t.Fatalf("idempotent directory: %#v, %v", result, err)
	}
	if _, err := session.CreateTextFile("created/file.txt", "before\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.CreateTextFile("created/file.txt", "overwrite\n"); !errors.Is(err, ErrPathExists) {
		t.Fatalf("create-only file overwrote existing file: %v", err)
	}
	if _, err := session.CreateTextFile("missing/file.txt", "no parent"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("create file created parents: %v", err)
	}

	old := "before\n"
	oldSum := sha256.Sum256([]byte(old))
	result, err := session.WriteTextFile("created/file.txt", hex.EncodeToString(oldSum[:]), "after\n")
	if err != nil || result.Status != "updated" || result.PreviousSHA256 != hex.EncodeToString(oldSum[:]) {
		t.Fatalf("hash update: %#v, %v", result, err)
	}
	if got, err := session.ReadText("created/file.txt"); err != nil || got != "after\n" {
		t.Fatalf("updated content: %q, %v", got, err)
	}
	if _, err := session.WriteTextFile("created/file.txt", hex.EncodeToString(oldSum[:]), "stale\n"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale hash accepted: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "created", "file.txt"), []byte("external\n"), 0600); err != nil {
		t.Fatal(err)
	}
	newSum := sha256.Sum256([]byte("after\n"))
	if _, err := session.WriteTextFile("created/file.txt", hex.EncodeToString(newSum[:]), "should-conflict\n"); !errors.Is(err, ErrConflict) {
		t.Fatalf("external change was not detected: %v", err)
	}
	if _, err := session.CreateTextFile("nul.txt", "a\x00b"); !errors.Is(err, ErrNotText) {
		t.Fatalf("NUL content accepted: %v", err)
	}
}
