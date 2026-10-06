package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/programming"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

const tunnelTestToken = "secure-mcp-tunnel-test-token-0123456789-abcdef"

type tunnelProgrammingStub struct {
	reads int
}

func (s *tunnelProgrammingStub) ReadText(string, string, string, string) (string, error) {
	s.reads++
	return "ok", nil
}

func (*tunnelProgrammingStub) ListDirectory(string, string, string, string) ([]string, error) {
	return []string{}, nil
}

func (*tunnelProgrammingStub) StatPath(string, string, string, string) (workspace.PathStat, error) {
	return workspace.PathStat{}, nil
}

func (*tunnelProgrammingStub) FindPaths(string, string, string, string, string, int, int) (workspace.FindResult, error) {
	return workspace.FindResult{}, nil
}

func (*tunnelProgrammingStub) SearchText(string, string, string, string, string, int) (workspace.SearchResult, error) {
	return workspace.SearchResult{}, nil
}

func (*tunnelProgrammingStub) ReplaceText(string, string, string, string, string, string) error {
	return nil
}

func (*tunnelProgrammingStub) CreateDirectory(string, string, string, string) (workspace.DirectoryResult, error) {
	return workspace.DirectoryResult{}, nil
}

func (*tunnelProgrammingStub) CreateTextFile(string, string, string, string, string) (workspace.TextFileResult, error) {
	return workspace.TextFileResult{}, nil
}

func (*tunnelProgrammingStub) WriteTextFile(string, string, string, string, string, string) (workspace.TextFileResult, error) {
	return workspace.TextFileResult{}, nil
}

func (*tunnelProgrammingStub) Copy(string, string, string, string, string) (workspace.CopyResult, error) {
	return workspace.CopyResult{}, nil
}

func (*tunnelProgrammingStub) Move(string, string, string, string, string) (workspace.MoveResult, error) {
	return workspace.MoveResult{}, nil
}

func (*tunnelProgrammingStub) DeleteFile(string, string, string, string) (workspace.DeleteResult, error) {
	return workspace.DeleteResult{}, nil
}

func (*tunnelProgrammingStub) DeleteDirectory(string, string, string, string) (workspace.DeleteResult, error) {
	return workspace.DeleteResult{}, nil
}

func (*tunnelProgrammingStub) ApplyPatch(string, string, string, []workspace.PatchOperation) (workspace.PatchResult, error) {
	return workspace.PatchResult{}, nil
}

func (*tunnelProgrammingStub) ReviewGit(context.Context, string, string, string) (programming.DiffReview, error) {
	return programming.DiffReview{}, nil
}

func (*tunnelProgrammingStub) GitStatus(context.Context, string, string, string) (workspace.GitIndexStatus, error) {
	return workspace.GitIndexStatus{}, nil
}

func (*tunnelProgrammingStub) StageGitPaths(context.Context, string, string, string, string, []workspace.GitIndexEntry) (workspace.GitIndexMutationResult, error) {
	return workspace.GitIndexMutationResult{}, nil
}

func (*tunnelProgrammingStub) UnstageGitPaths(context.Context, string, string, string, string, []workspace.GitIndexEntry) (workspace.GitIndexMutationResult, error) {
	return workspace.GitIndexMutationResult{}, nil
}

func (*tunnelProgrammingStub) CommitGitIndex(context.Context, string, string, string, workspace.GitCommitRequest) (workspace.GitCommitResult, error) {
	return workspace.GitCommitResult{}, nil
}

func tunnelTestPorts(stub *tunnelProgrammingStub) ProgrammingPorts {
	return ProgrammingPorts{
		WorkspaceReader: stub, WorkspaceLister: stub, WorkspaceStatter: stub,
		WorkspaceFinder: stub, WorkspaceSearcher: stub, WorkspaceWriter: stub,
		WorkspaceDirectoryCreator: stub, WorkspaceTextCreator: stub, WorkspaceTextUpdater: stub,
		WorkspaceCopier: stub, WorkspaceMover: stub, WorkspaceFileDeleter: stub,
		WorkspaceDirectoryDeleter: stub, WorkspacePatchApplier: stub,
		GitReviewer: stub, GitStatusReader: stub, GitIndexer: stub, GitCommitter: stub,
	}
}

type tunnelAuthorizerSpy struct {
	calls int
	input ProgrammingAuthorizationInput
	err   error
}

func (s *tunnelAuthorizerSpy) Authorize(_ context.Context, input ProgrammingAuthorizationInput) error {
	s.calls++
	s.input = input
	return s.err
}

func newTunnelTestHandler(t *testing.T, authorizer ProgrammingAuthorizer) (http.Handler, *tunnelProgrammingStub) {
	t.Helper()
	stub := &tunnelProgrammingStub{}
	handler, err := NewTunnelProgrammingHandler(TunnelProgrammingConfig{
		OwnerSubject: "owner-test",
		ClientID:     strings.Repeat("a", 32),
		Token:        tunnelTestToken,
		Port:         7676,
		ProgrammingAuthorization: func() ProgrammingAuthorizer {
			return authorizer
		},
	}, tunnelTestPorts(stub))
	if err != nil {
		t.Fatal(err)
	}
	return handler, stub
}

func tunnelMCPRequest(t *testing.T, handler http.Handler, body string, tokenValues []string, host, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7676/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	for _, token := range tokenValues {
		req.Header.Add(TunnelProgrammingHeader, token)
	}
	if host != "" {
		req.Host = host
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestTunnelProgrammingAuthenticationBoundary(t *testing.T) {
	handler, _ := newTunnelTestHandler(t, &tunnelAuthorizerSpy{})
	body := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}"
	cases := []struct {
		name   string
		tokens []string
		host   string
		origin string
		status int
	}{
		{name: "missing token", status: http.StatusUnauthorized},
		{name: "wrong token", tokens: []string{"wrong-tunnel-token-with-enough-length-0123456789"}, status: http.StatusUnauthorized},
		{name: "duplicate token", tokens: []string{tunnelTestToken, tunnelTestToken}, status: http.StatusUnauthorized},
		{name: "wrong host", tokens: []string{tunnelTestToken}, host: "attacker.example", status: http.StatusForbidden},
		{name: "forwarded origin with valid local credential", tokens: []string{tunnelTestToken}, origin: "https://chatgpt.com", status: http.StatusOK},
		{name: "valid", tokens: []string{tunnelTestToken}, status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := tunnelMCPRequest(t, handler, body, tc.tokens, tc.host, tc.origin)
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s want=%d", rec.Code, rec.Body.String(), tc.status)
			}
			if tc.status == http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate") != "" {
				t.Fatalf("Tunnel auth must not advertise OAuth: %q", rec.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

func TestTunnelProgrammingDiscoveryHasNoOAuthSecuritySchemes(t *testing.T) {
	handler, _ := newTunnelTestHandler(t, &tunnelAuthorizerSpy{})
	rec := tunnelMCPRequest(t, handler, "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}", []string{tunnelTestToken}, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Result.Tools) != 20 {
		t.Fatalf("expected 20 tools, got %d", len(payload.Result.Tools))
	}
	for _, tool := range payload.Result.Tools {
		if _, exists := tool["securitySchemes"]; exists {
			t.Fatalf("Tunnel tool %q advertised OAuth securitySchemes", tool["name"])
		}
	}

	initReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7676/mcp", strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-06-18\"}}"))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set(TunnelProgrammingHeader, tunnelTestToken)
	initRec := httptest.NewRecorder()
	handler.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK || !strings.Contains(initRec.Body.String(), "Secure MCP Tunnel") || strings.Contains(initRec.Body.String(), "one OAuth Programming connection") {
		t.Fatalf("unexpected Tunnel initialize response: %d %s", initRec.Code, initRec.Body.String())
	}

	diag := tunnelMCPRequest(t, handler, "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"connection_diagnostic\",\"arguments\":{}}}", []string{tunnelTestToken}, "", "")
	if diag.Code != http.StatusOK || !strings.Contains(diag.Body.String(), "\"mode\":\"tunnel_programming\"") {
		t.Fatalf("unexpected Tunnel diagnostic: %d %s", diag.Code, diag.Body.String())
	}
}

func TestTunnelProgrammingUsesStaticPrincipalAndLocalAuthorizer(t *testing.T) {
	spy := &tunnelAuthorizerSpy{}
	handler, stub := newTunnelTestHandler(t, spy)
	body := "{\"jsonrpc\":\"2.0\",\"id\":4,\"method\":\"tools/call\",\"params\":{\"name\":\"read_file\",\"arguments\":{\"session_id\":\"session-1\",\"path\":\"file.txt\"}}}"
	rec := tunnelMCPRequest(t, handler, body, []string{tunnelTestToken}, "", "")
	if rec.Code != http.StatusOK || stub.reads != 1 {
		t.Fatalf("read call did not execute exactly once: status=%d reads=%d body=%s", rec.Code, stub.reads, rec.Body.String())
	}
	if spy.calls != 1 || spy.input.Identity.OwnerSubject != "owner-test" || spy.input.Identity.ClientID != strings.Repeat("a", 32) {
		t.Fatalf("unexpected Tunnel principal: calls=%d input=%+v", spy.calls, spy.input)
	}
	if spy.input.Tool != readToolName || spy.input.SessionID != "session-1" {
		t.Fatalf("unexpected authorization input: %+v", spy.input)
	}
}

func TestTunnelProgrammingFailsClosedWithoutLocalAuthorizer(t *testing.T) {
	handler, stub := newTunnelTestHandler(t, nil)
	body := "{\"jsonrpc\":\"2.0\",\"id\":5,\"method\":\"tools/call\",\"params\":{\"name\":\"read_file\",\"arguments\":{\"session_id\":\"session-1\",\"path\":\"file.txt\"}}}"
	rec := tunnelMCPRequest(t, handler, body, []string{tunnelTestToken}, "", "")
	if rec.Code != http.StatusOK || stub.reads != 0 || !strings.Contains(rec.Body.String(), "LOCAL_APPROVAL_UNAVAILABLE") {
		t.Fatalf("Tunnel call did not fail closed: status=%d reads=%d body=%s", rec.Code, stub.reads, rec.Body.String())
	}
}
