package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestStructuralCopyMoveAndDeleteRespectTypedBoundaries(t *testing.T) {
	session, root := approvedSession(t)
	if err := os.Mkdir(filepath.Join(root, "source"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "source", "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "nested", "file.txt"), []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}

	copied, err := session.Copy("source", "copied")
	if err != nil || copied.Status != "copied" || copied.Kind != FileKindDirectory || copied.FilesCopied != 1 || copied.EntriesCopied != 3 || copied.BytesCopied != int64(len("payload")) {
		t.Fatalf("copy result: %#v, %v", copied, err)
	}
	if info, err := os.Stat(filepath.Join(root, "copied", "nested", "file.txt")); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("copy permission/stat: %v, %v", info, err)
	}
	if info, err := os.Stat(filepath.Join(root, "copied", "nested")); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("copy directory permission/stat: %v, %v", info, err)
	}
	if _, err := session.Copy("source", "copied"); !errors.Is(err, ErrPathExists) {
		t.Fatalf("copy overwrote destination: %v", err)
	}

	moved, err := session.Move("copied/nested/file.txt", "moved.txt")
	if err != nil || moved.Status != "moved" || moved.Kind != FileKindRegular {
		t.Fatalf("move result: %#v, %v", moved, err)
	}
	if _, err := os.Stat(filepath.Join(root, "moved.txt")); err != nil {
		t.Fatalf("moved file missing: %v", err)
	}
	if _, err := session.Move("source", "source/nested/child"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("directory moved into child: %v", err)
	}

	deleted, err := session.DeleteFile("moved.txt")
	if err != nil || deleted.Status != "deleted" || deleted.Kind != FileKindRegular {
		t.Fatalf("delete file result: %#v, %v", deleted, err)
	}
	if _, err := session.DeleteDirectory("source"); !errors.Is(err, ErrDirectoryNotEmpty) {
		t.Fatalf("non-empty directory deleted: %v", err)
	}
	if _, err := session.DeleteDirectory("."); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("root directory deleted: %v", err)
	}
	if _, err := session.DeleteDirectory("source/nested"); !errors.Is(err, ErrDirectoryNotEmpty) {
		t.Fatalf("nested non-empty directory deleted: %v", err)
	}
	if _, err := session.DeleteFile("source/nested/file.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.DeleteDirectory("source/nested"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.DeleteDirectory("source"); err != nil {
		t.Fatal(err)
	}
}

func TestStructuralOperationsRejectUnsafeTypesAndTraversal(t *testing.T) {
	session, root := approvedSession(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Copy("link", "copy"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("copied source symlink: %v", err)
	}
	if _, err := session.Copy(".", "copy"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("copied workspace root: %v", err)
	}
	if _, err := session.Move("link", "moved"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("moved source symlink: %v", err)
	}
	if _, err := session.DeleteFile("link"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("deleted symlink as file: %v", err)
	}
	if _, err := session.DeleteFile("../secret"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("accepted traversal: %v", err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Copy("fifo", "fifo-copy"); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("copied special file: %v", err)
	}
}

func TestStructuralCopyLimitsAndPartialCleanup(t *testing.T) {
	session, root := approvedSession(t)
	if err := os.WriteFile(filepath.Join(root, "too-large"), []byte(strings.Repeat("x", MaxStructuralBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Copy("too-large", "large-copy"); !errors.Is(err, ErrStructuralLimit) {
		t.Fatalf("size limit was not enforced: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "large-copy")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("limited file left a destination: %v", err)
	}

	if err := os.Mkdir(filepath.Join(root, "partial"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "partial", "ok.txt"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "partial", "ok.txt"), filepath.Join(root, "partial", "unsafe")); err != nil {
		t.Fatal(err)
	}
	result, err := session.Copy("partial", "partial-copy")
	if !errors.Is(err, ErrUnsafePath) || result.Status != "failed" || result.Cleanup != "confirmed" || result.Partial {
		t.Fatalf("partial cleanup result: %#v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, "partial-copy")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial destination was not cleaned: %v", err)
	}

	deep := filepath.Join(root, "deep")
	if err := os.Mkdir(deep, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= MaxStructuralDepth; i++ {
		deep = filepath.Join(deep, "d")
		if err := os.Mkdir(deep, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := session.Copy("deep", "deep-copy"); !errors.Is(err, ErrStructuralLimit) {
		t.Fatalf("depth limit was not enforced: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "deep-copy")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("depth-limited destination was not cleaned: %v", err)
	}
}

func TestStructuralEntryLimitAndGrantBoundary(t *testing.T) {
	session, root := approvedSession(t)
	if err := os.Mkdir(filepath.Join(root, "many"), 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxStructuralEntries; i++ {
		name := filepath.Join(root, "many", fmt.Sprintf("%04d", i))
		if err := os.WriteFile(name, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := session.Copy("many", "many-copy"); !errors.Is(err, ErrStructuralLimit) {
		t.Fatalf("entry limit was not enforced: %v", err)
	}

	grants, err := NewGrants("owner")
	if err != nil {
		t.Fatal(err)
	}
	client := strings.Repeat("A", 32)
	id, err := grants.GrantWithScopes(root, client, ScopeRead, ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := grants.Copy("wrong-owner", client, id, "many", "wrong-owner-copy"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("copy accepted owner divergence: %v", err)
	}
	if _, err := grants.Copy("owner", strings.Repeat("B", 32), id, "many", "wrong-client-copy"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("copy accepted client divergence: %v", err)
	}
	if _, err := grants.Copy("owner", client, strings.Repeat("0", 32), "many", "wrong-session-copy"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("copy accepted session divergence: %v", err)
	}
	if err := grants.Revoke(id); err != nil {
		t.Fatal(err)
	}
	if _, err := grants.DeleteFile("owner", client, id, "many/0000"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("delete accepted revoked grant: %v", err)
	}
}
