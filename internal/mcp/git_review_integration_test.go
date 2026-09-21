package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/programming"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

type harnessGitReviewer struct {
	grants      *workspace.Grants
	before      programming.GitSnapshot
	outputLimit int
	starts      *atomic.Int32
}

func (r *harnessGitReviewer) ReviewGit(ctx context.Context, owner, clientID, sessionID string) (programming.DiffReview, error) {
	var after programming.GitSnapshot
	err := r.grants.WithAuthorizedGitProcessDir(owner, clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		if r.starts != nil {
			r.starts.Add(1)
		}
		var err error
		after, err = programming.CaptureGitSnapshot(ctx, directory, r.outputLimit)
		return err
	})
	if err != nil {
		return programming.DiffReview{}, err
	}
	return programming.CompareGitSnapshots(r.before, after), nil
}

func TestGitReviewMCPVerticalFlowUsesIndependentScopeAndRevoke(t *testing.T) {
	key, verifier, _, _ := setupOAuth(t)
	root := gitReviewFixture(t)
	grants, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grants.Close() })
	clientID := strings.Repeat("G", 32)
	sessionID, err := grants.GrantWithScopes(root, clientID, workspace.ScopeRead, workspace.ScopeWrite, workspace.ScopeGit)
	if err != nil {
		t.Fatal(err)
	}
	var baseline programming.GitSnapshot
	err = grants.WithAuthorizedGitProcessDir("owner-test", clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		var err error
		baseline, err = programming.CaptureGitSnapshot(context.Background(), directory, 8192)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewOAuthHandler(OAuthConfig{
		ResourceURL:     testResource,
		Issuer:          testIssuer,
		OwnerSubject:    "owner-test",
		workspaceWriter: grants,
		gitReviewer:     &harnessGitReviewer{grants: grants, before: baseline, outputLimit: 8192},
	}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	writeToken := signedWriteToken(t, key, clientID, diagnosticScope+" "+workspaceWriteScope)
	gitToken := signedWriteToken(t, key, clientID, diagnosticScope+" "+gitReviewScope)
	listResponse, list := oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if listResponse.StatusCode != http.StatusOK {
		t.Fatalf("write-only tools/list failed: %d", listResponse.StatusCode)
	}
	if names := toolNames(t, list); len(names) != 2 || names[1] != writeToolName {
		t.Fatalf("Git tool advertised without Git scope: %v", names)
	}
	listResponse, list = oauthRequest(t, server, http.MethodPost, "/mcp", gitToken, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, nil)
	if listResponse.StatusCode != http.StatusOK {
		t.Fatalf("Git-only tools/list failed: %d", listResponse.StatusCode)
	}
	if names := toolNames(t, list); len(names) != 2 || names[1] != gitReviewToolName {
		t.Fatalf("Git tool was not discovered with its independent scope: %v", names)
	}
	for _, testCase := range []struct {
		name  string
		token string
		want  string
	}{
		{name: "read-only scope", token: signedWriteToken(t, key, clientID, diagnosticScope+" "+workspaceReadScope), want: "Git review authorization required."},
		{name: "write-only scope", token: writeToken, want: "Git review authorization required."},
		{name: "client mismatch", token: signedWriteToken(t, key, strings.Repeat("H", 32), diagnosticScope+" "+gitReviewScope), want: "Git review unavailable or not authorized."},
		{name: "session mismatch", token: gitToken, want: "Git review unavailable or not authorized."},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			session := sessionID
			if testCase.name == "session mismatch" {
				session = strings.Repeat("s", len(sessionID))
			}
			res, result := oauthRequest(t, server, http.MethodPost, "/mcp", testCase.token, gitReviewCall(session), nil)
			text, isError := readResult(t, result)
			if res.StatusCode != http.StatusOK || !isError || text != testCase.want {
				t.Fatalf("unexpected Git authorization result: %d %v", res.StatusCode, result)
			}
		})
	}
	missingTokenResponse, _ := oauthRequest(t, server, http.MethodPost, "/mcp", "", gitReviewCall(sessionID), nil)
	if missingTokenResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing token reached Git tool: %d", missingTokenResponse.StatusCode)
	}
	invalidTokenResponse, _ := oauthRequest(t, server, http.MethodPost, "/mcp", "not-a-jwt-token-with-enough-length-0123456789", gitReviewCall(sessionID), nil)
	if invalidTokenResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid token reached Git tool: %d", invalidTokenResponse.StatusCode)
	}
	otherOwnerClaims := defaultClaims()
	otherOwnerClaims["sub"] = "other-owner"
	otherOwnerClaims["client_id"] = clientID
	otherOwnerClaims["scope"] = diagnosticScope + " " + gitReviewScope
	otherOwnerResponse, _ := oauthRequest(t, server, http.MethodPost, "/mcp", makeAccessToken(t, key, otherOwnerClaims), gitReviewCall(sessionID), nil)
	if otherOwnerResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("other owner reached Git tool: %d", otherOwnerResponse.StatusCode)
	}
	extraParams, _ := json.Marshal(map[string]any{
		"name":      gitReviewToolName,
		"arguments": map[string]any{"session_id": sessionID, "extra": true},
	})
	extraRequest, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": json.RawMessage(extraParams)})
	extraResponse, extraResult := oauthRequest(t, server, http.MethodPost, "/mcp", gitToken, string(extraRequest), nil)
	if extraResponse.StatusCode != http.StatusOK || extraResult["error"] == nil {
		t.Fatalf("extra Git argument was accepted: %d %v", extraResponse.StatusCode, extraResult)
	}

	editResponse, edit := oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, writeCall(sessionID, "editable.txt", "before\n", "after\n"), nil)
	if editResponse.StatusCode != http.StatusOK || edit["result"].(map[string]any)["isError"] != false || fileText(t, filepath.Join(root, "editable.txt")) != "after\n" {
		t.Fatalf("MCP edit failed before Git review: %d %v", editResponse.StatusCode, edit)
	}

	reviewResponse, reviewResult := oauthRequest(t, server, http.MethodPost, "/mcp", gitToken, gitReviewCall(sessionID), nil)
	reviewText, reviewError := readResult(t, reviewResult)
	if reviewResponse.StatusCode != http.StatusOK || reviewError {
		t.Fatalf("authorized Git review failed: %d %v", reviewResponse.StatusCode, reviewResult)
	}
	var review struct {
		After        struct{ Status, Diff string } `json:"after"`
		Complete     bool                          `json:"complete"`
		StatusChange bool                          `json:"status_changed"`
		DiffChange   bool                          `json:"diff_changed"`
	}
	if err := json.Unmarshal([]byte(reviewText), &review); err != nil {
		t.Fatal(err)
	}
	if !review.Complete || !review.StatusChange || !review.DiffChange || !strings.Contains(review.After.Status, " M editable.txt") || !strings.Contains(review.After.Diff, "-before") || !strings.Contains(review.After.Diff, "+after") {
		t.Fatalf("Git review did not expose the produced edit: %+v", review)
	}

	deniedResponse, deniedResult := oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, gitReviewCall(sessionID), nil)
	deniedText, deniedError := readResult(t, deniedResult)
	if deniedResponse.StatusCode != http.StatusOK || !deniedError || deniedText != "Git review authorization required." {
		t.Fatalf("missing Git scope was not denied independently: %d %v", deniedResponse.StatusCode, deniedResult)
	}

	if err := grants.Revoke(sessionID); err != nil {
		t.Fatal(err)
	}
	revokedResponse, revokedResult := oauthRequest(t, server, http.MethodPost, "/mcp", gitToken, gitReviewCall(sessionID), nil)
	revokedText, revokedError := readResult(t, revokedResult)
	if revokedResponse.StatusCode != http.StatusOK || !revokedError || revokedText != "Git review unavailable or not authorized." {
		t.Fatalf("revoked Git grant remained usable: %d %v", revokedResponse.StatusCode, revokedResult)
	}
}

func gitReviewCall(sessionID string) string {
	params, _ := json.Marshal(map[string]any{
		"name":      gitReviewToolName,
		"arguments": map[string]string{"session_id": sessionID},
	})
	request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": json.RawMessage(params)})
	return string(request)
}

func gitReviewFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitReviewFixture(t, root, "init", "-q")
	runGitReviewFixture(t, root, "config", "user.email", "fixture@example.invalid")
	runGitReviewFixture(t, root, "config", "user.name", "SignalSpace fixture")
	if err := os.WriteFile(filepath.Join(root, "editable.txt"), []byte("before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitReviewFixture(t, root, "add", "editable.txt")
	runGitReviewFixture(t, root, "commit", "-qm", "baseline")
	return root
}

func runGitReviewFixture(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
