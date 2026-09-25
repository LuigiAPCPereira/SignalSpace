package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

type fakeGitCommitter struct {
	owner, client, session string
	request                workspace.GitCommitRequest
}

func (f *fakeGitCommitter) CommitGitIndex(_ context.Context, owner, client, session string, request workspace.GitCommitRequest) (workspace.GitCommitResult, error) {
	f.owner, f.client, f.session, f.request = owner, client, session, request
	return workspace.GitCommitResult{Status: "committed", CommitOID: "0123456789012345678901234567890123456789", ParentOID: request.ExpectedHeadOID, TreeOID: "0123456789012345678901234567890123456789", PreviousHeadOID: request.ExpectedHeadOID, CurrentHeadOID: "0123456789012345678901234567890123456789", Detached: true, ReachabilityState: "private_ref_and_head_confirmed"}, nil
}

func TestGitCommitToolIsIndependentAndClosed(t *testing.T) {
	key, verifier, _, _ := setupOAuth(t)
	committer := &fakeGitCommitter{}
	handler, err := NewOAuthHandler(OAuthConfig{
		ResourceURL:  testResource,
		Issuer:       testIssuer,
		OwnerSubject: "owner-test",
		gitCommitter: committer,
	}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	clientID := strings.Repeat("C", 32)
	commitToken := signedWriteToken(t, key, clientID, diagnosticScope+" "+gitCommitScope)
	reviewToken := signedWriteToken(t, key, clientID, diagnosticScope+" "+gitReviewScope)
	commitServer := newTestServer(t, handler)
	_, list := oauthRequest(t, commitServer, http.MethodPost, "/mcp", commitToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	names := toolNames(t, list)
	if len(names) != 2 || names[1] != commitGitIndexToolName {
		t.Fatalf("unexpected commit-only tools: %v", names)
	}
	_, list = oauthRequest(t, commitServer, http.MethodPost, "/mcp", reviewToken, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, nil)
	if names := toolNames(t, list); len(names) != 1 {
		t.Fatalf("commit leaked to review token: %v", names)
	}
	call := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"commit_git_index","arguments":{"session_id":"session","expected_head_oid":"0123456789012345678901234567890123456789","expected_index_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","message":"local commit"}}}`
	_, result := oauthRequest(t, commitServer, http.MethodPost, "/mcp", commitToken, call, nil)
	text, isError := readResult(t, result)
	if isError || text == "" || committer.owner != "owner-test" || committer.client != clientID || committer.session != "session" || committer.request.Message != "local commit" {
		t.Fatalf("commit call failed: error=%t text=%q committer=%+v", isError, text, committer)
	}
	bad := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"commit_git_index","arguments":{"session_id":"session","expected_head_oid":"0123456789012345678901234567890123456789","expected_index_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","message":"local commit","author":"forbidden"}}}`
	response, _ := oauthRequest(t, commitServer, http.MethodPost, "/mcp", commitToken, bad, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("extra argument did not return JSON-RPC invalid params: %d", response.StatusCode)
	}
}

func newTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}
