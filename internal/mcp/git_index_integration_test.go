package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

type harnessGitIndex struct{ grants *workspace.Grants }

func (h *harnessGitIndex) GitStatus(_ context.Context, owner, clientID, sessionID string) (workspace.GitIndexStatus, error) {
	var status workspace.GitIndexStatus
	err := h.grants.WithAuthorizedGitProcessDir(owner, clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		var err error
		status, err = workspace.CaptureGitIndexStatus(directory)
		return err
	})
	return status, err
}

func (h *harnessGitIndex) StageGitPaths(_ context.Context, owner, clientID, sessionID, expected string, entries []workspace.GitIndexEntry) (workspace.GitIndexMutationResult, error) {
	var result workspace.GitIndexMutationResult
	err := h.grants.WithAuthorizedManagedGitProcessDir(owner, clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		var err error
		result, err = workspace.StageGitPaths(directory, expected, entries)
		return err
	})
	return result, err
}

func (h *harnessGitIndex) UnstageGitPaths(_ context.Context, owner, clientID, sessionID, expected string, entries []workspace.GitIndexEntry) (workspace.GitIndexMutationResult, error) {
	var result workspace.GitIndexMutationResult
	err := h.grants.WithAuthorizedManagedGitProcessDir(owner, clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		var err error
		result, err = workspace.UnstageGitPaths(directory, expected, entries)
		return err
	})
	return result, err
}

func TestGitIndexMCPKeepsReviewAndIndexScopesIndependent(t *testing.T) {
	key, verifier, _, _ := setupOAuth(t)
	source := gitReviewFixture(t)
	state := filepath.Join(t.TempDir(), "state")
	manager, err := workspace.NewManagedWorktreeManager(state)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	descriptor, err := manager.Create(source, "")
	if err != nil {
		t.Fatal(err)
	}
	grants, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	clientID := strings.Repeat("I", 32)
	sessionID, _, err := manager.Activate(descriptor.WorkspaceID, clientID, grants, workspace.ScopeRead, workspace.ScopeGit, workspace.ScopeGitIndex)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = grants.Revoke(sessionID)
		_ = manager.Deactivate(descriptor.WorkspaceID)
	}()
	harness := &harnessGitIndex{grants: grants}
	handler, err := NewOAuthHandler(OAuthConfig{
		ResourceURL:     testResource,
		Issuer:          testIssuer,
		OwnerSubject:    "owner-test",
		gitStatusReader: harness,
		gitIndexer:      harness,
	}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	allToken := signedWriteToken(t, key, clientID, diagnosticScope+" "+gitReviewScope+" "+gitIndexScope)
	reviewOnlyToken := signedWriteToken(t, key, clientID, diagnosticScope+" "+gitReviewScope)

	_, list := oauthRequest(t, server, http.MethodPost, "/mcp", allToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	names := toolNames(t, list)
	if len(names) != 4 || names[1] != gitStatusToolName || names[2] != stageGitPathsName || names[3] != unstageGitPathsName {
		t.Fatalf("unexpected isolated Git index surface: %v", names)
	}
	_, reviewList := oauthRequest(t, server, http.MethodPost, "/mcp", reviewOnlyToken, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, nil)
	if names := toolNames(t, reviewList); len(names) != 2 || names[1] != gitStatusToolName {
		t.Fatalf("Git index leaked into review-only token: %v", names)
	}

	managedRoot := filepath.Join(state, "worktrees", descriptor.WorkspaceID)
	if err := os.WriteFile(filepath.Join(managedRoot, "editable.txt"), []byte("after\n"), 0600); err != nil {
		t.Fatal(err)
	}
	statusResponse, statusResult := oauthRequest(t, server, http.MethodPost, "/mcp", allToken, gitIndexStatusCall(sessionID), nil)
	statusText, statusError := readResult(t, statusResult)
	if statusResponse.StatusCode != http.StatusOK || statusError {
		t.Fatalf("Git status failed: %d %v", statusResponse.StatusCode, statusResult)
	}
	var status workspace.GitIndexStatus
	if err := json.Unmarshal([]byte(statusText), &status); err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256([]byte("after\n"))
	stageResponse, stageResult := oauthRequest(t, server, http.MethodPost, "/mcp", allToken, gitIndexMutationCall(stageGitPathsName, sessionID, status.IndexSHA256, []map[string]any{{"path": "editable.txt", "expected_sha256": hex.EncodeToString(expected[:])}}), nil)
	stageText, stageError := readResult(t, stageResult)
	if stageResponse.StatusCode != http.StatusOK || stageError || !strings.Contains(stageText, `"status":"staged"`) {
		t.Fatalf("Git stage failed: %d %v %q", stageResponse.StatusCode, stageResult, stageText)
	}

	_, stagedStatusResult := oauthRequest(t, server, http.MethodPost, "/mcp", allToken, gitIndexStatusCall(sessionID), nil)
	stagedStatusText, stagedStatusError := readResult(t, stagedStatusResult)
	if stagedStatusError {
		t.Fatalf("staged Git status failed: %v", stagedStatusResult)
	}
	var stagedStatus workspace.GitIndexStatus
	if err := json.Unmarshal([]byte(stagedStatusText), &stagedStatus); err != nil {
		t.Fatal(err)
	}
	var stagedEntry workspace.GitIndexStatusEntry
	for _, entry := range stagedStatus.Entries {
		if entry.Path == "editable.txt" {
			stagedEntry = entry
		}
	}
	if stagedEntry.IndexObjectID == "" || !stagedEntry.Staged {
		t.Fatalf("staged status missing index object: %+v", stagedEntry)
	}
	unstageResponse, unstageResult := oauthRequest(t, server, http.MethodPost, "/mcp", allToken, gitIndexMutationCall(unstageGitPathsName, sessionID, stagedStatus.IndexSHA256, []map[string]any{{"path": "editable.txt", "expected_index_oid": stagedEntry.IndexObjectID}}), nil)
	unstageText, unstageError := readResult(t, unstageResult)
	if unstageResponse.StatusCode != http.StatusOK || unstageError || !strings.Contains(unstageText, `"status":"unstaged"`) {
		t.Fatalf("Git unstage failed: %d %v %q", unstageResponse.StatusCode, unstageResult, unstageText)
	}
}

func gitIndexStatusCall(sessionID string) string {
	params, _ := json.Marshal(map[string]any{"name": gitStatusToolName, "arguments": map[string]string{"session_id": sessionID}})
	request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": json.RawMessage(params)})
	return string(request)
}

func gitIndexMutationCall(name, sessionID, expected string, entries []map[string]any) string {
	params, _ := json.Marshal(map[string]any{"name": name, "arguments": map[string]any{"session_id": sessionID, "expected_index_sha256": expected, "entries": entries}})
	request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": json.RawMessage(params)})
	return string(request)
}
