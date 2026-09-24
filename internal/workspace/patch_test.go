package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyPatchPreflightsAndCompensatesStructuredOperations(t *testing.T) {
	session, root := approvedSession(t)
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	oldHash := sha256.Sum256([]byte("before"))
	result, err := session.ApplyPatch([]PatchOperation{
		{Type: PatchCreateDirectory, Path: "generated"},
		{Type: PatchCreateFile, Path: "generated/new.txt", Content: "new"},
		{Type: PatchWriteFile, Path: "tracked.txt", ExpectedSHA256: hex.EncodeToString(oldHash[:]), Content: "after"},
	})
	if err != nil || result.Status != "applied" || result.OperationsApplied != 3 || result.Summary.Created != 2 || result.Summary.Updated != 1 {
		t.Fatalf("applied patch: %#v, %v", result, err)
	}
	if got, err := session.ReadText("tracked.txt"); err != nil || got != "after" {
		t.Fatalf("updated content: %q, %v", got, err)
	}
	if got, err := session.ReadText("generated/new.txt"); err != nil || got != "new" {
		t.Fatalf("created content: %q, %v", got, err)
	}

	stale, err := session.ApplyPatch([]PatchOperation{{Type: PatchWriteFile, Path: "tracked.txt", ExpectedSHA256: hex.EncodeToString(oldHash[:]), Content: "stale"}})
	if !errors.Is(err, ErrPatchHashConflict) || stale.Status != "hash_conflict" || stale.OperationsApplied != 0 {
		t.Fatalf("stale preflight: %#v, %v", stale, err)
	}
	if got, _ := session.ReadText("tracked.txt"); got != "after" {
		t.Fatalf("stale patch mutated content: %q", got)
	}

	createdHash := sha256.Sum256([]byte("new"))
	deleteResult, err := session.ApplyPatch([]PatchOperation{{Type: PatchDeleteFile, Path: "generated/new.txt", ExpectedSHA256: hex.EncodeToString(createdHash[:])}})
	if err != nil || deleteResult.Status != "applied" || deleteResult.Summary.Deleted != 1 {
		t.Fatalf("delete patch: %#v, %v", deleteResult, err)
	}
	if stat, err := session.StatPath("generated/new.txt"); err != nil || stat.Found {
		t.Fatalf("deleted file still visible: %#v, %v", stat, err)
	}
}

func TestApplyPatchRejectsConflictingOperationsWithoutMutation(t *testing.T) {
	session, root := approvedSession(t)
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := session.ApplyPatch([]PatchOperation{
		{Type: PatchCreateFile, Path: "new.txt", Content: "new"},
		{Type: PatchCreateFile, Path: "new.txt", Content: "different"},
	})
	if !errors.Is(err, ErrPatchConflict) || result.Status != "conflict" || result.OperationsApplied != 0 {
		t.Fatalf("conflicting patch: %#v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight failure mutated destination: %v", err)
	}
	invalid, err := session.ApplyPatch([]PatchOperation{{Type: PatchCreateFile, Path: "/absolute.txt", Content: "no"}})
	if !errors.Is(err, ErrInvalidPath) || invalid.Status != "invalid_path" || invalid.OperationsApplied != 0 {
		t.Fatalf("invalid path status: %#v, %v", invalid, err)
	}
}

func TestApplyPatchPreflightsExistingChildUnderExistingDirectory(t *testing.T) {
	session, root := approvedSession(t)
	if err := os.Mkdir(filepath.Join(root, "existing"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "existing", "file.txt"), []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := session.ApplyPatch([]PatchOperation{
		{Type: PatchCreateDirectory, Path: "existing"},
		{Type: PatchCreateFile, Path: "existing/file.txt", Content: "overwrite"},
	})
	if !errors.Is(err, ErrPatchAlreadyExists) || result.Status != "already_exists" || result.OperationsApplied != 0 {
		t.Fatalf("existing child was not rejected during preflight: %#v, %v", result, err)
	}
	if got, readErr := os.ReadFile(filepath.Join(root, "existing", "file.txt")); readErr != nil || string(got) != "before" {
		t.Fatalf("preflight failure mutated existing child: %q, %v", got, readErr)
	}
}
