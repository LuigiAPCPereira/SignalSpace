package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

const gitCommitScope = workspace.ScopeGitCommit

// WorkspaceGitCommitter é uma porta dedicada: não expõe raiz, comando,
// identidade, parent, ref, branch, hooks, signing ou Git remoto ao transporte.
type WorkspaceGitCommitter interface {
	CommitGitIndex(context.Context, string, string, string, workspace.GitCommitRequest) (workspace.GitCommitResult, error)
}

type gitCommitToolAccess struct {
	committer    WorkspaceGitCommitter
	verify       func(context.Context) (VerifiedIdentity, error)
	challenge    string
	discoverable bool
}

func commitGitIndexDefinition() map[string]any {
	return map[string]any{
		"name":        commitGitIndexToolName,
		"description": "Create one local staged-only commit in the approved managed worktree. Detached HEAD and private SignalSpace ref are advanced atomically; no hooks, signing, push, branch or shell.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":            map[string]any{"type": "string"},
				"expected_head_oid":     map[string]any{"type": "string", "minLength": 40, "maxLength": 64},
				"expected_index_sha256": map[string]any{"type": "string", "minLength": 64, "maxLength": 64},
				"message":               map[string]any{"type": "string", "maxLength": workspace.MaxGitCommitMessageBytes},
			},
			"required":             []string{"session_id", "expected_head_oid", "expected_index_sha256", "message"},
			"additionalProperties": false,
		},
		"securitySchemes": oauthSecuritySchemes(gitCommitScope),
		"annotations":     map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false},
	}
}

func (a *gitCommitToolAccess) call(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, err := a.verify(ctx)
	if err != nil {
		challenge := a.challenge + `, error="invalid_token", error_description="Invalid access token"`
		if errors.Is(err, ErrInsufficientScope) {
			challenge = a.challenge + `, error="insufficient_scope", error_description="Git commit scope is required"`
		}
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Git commit authorization required."}},
			"_meta":   map[string]any{"mcp/www_authenticate": []string{challenge}},
			"isError": true,
		}})
		return
	}
	request, ok := decodeGitCommitArguments(raw)
	if !ok {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	result, err := a.committer.CommitGitIndex(ctx, identity.OwnerSubject, identity.ClientID, request.SessionID, workspace.GitCommitRequest{
		ExpectedHeadOID: request.ExpectedHeadOID, ExpectedIndexSHA256: request.ExpectedIndexSHA256, Message: request.Message,
	})
	if err != nil {
		payload, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			fail(w, http.StatusOK, id, -32603, "Git commit unavailable")
			return
		}
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": string(payload)}},
			"isError": true,
		}})
		return
	}
	payload, err := json.Marshal(result)
	if err != nil {
		fail(w, http.StatusOK, id, -32603, "Git commit unavailable")
		return
	}
	reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(payload)}},
		"isError": false,
	}})
}

type gitCommitArguments struct {
	SessionID           string
	ExpectedHeadOID     string
	ExpectedIndexSHA256 string
	Message             string
}

func decodeGitCommitArguments(raw json.RawMessage) (gitCommitArguments, bool) {
	var arguments map[string]json.RawMessage
	if json.Unmarshal(raw, &arguments) != nil || len(arguments) != 4 {
		return gitCommitArguments{}, false
	}
	for key := range arguments {
		if key != "session_id" && key != "expected_head_oid" && key != "expected_index_sha256" && key != "message" {
			return gitCommitArguments{}, false
		}
	}
	var result gitCommitArguments
	if json.Unmarshal(arguments["session_id"], &result.SessionID) != nil || result.SessionID == "" ||
		json.Unmarshal(arguments["expected_head_oid"], &result.ExpectedHeadOID) != nil ||
		json.Unmarshal(arguments["expected_index_sha256"], &result.ExpectedIndexSHA256) != nil ||
		json.Unmarshal(arguments["message"], &result.Message) != nil || strings.TrimSpace(result.ExpectedHeadOID) != result.ExpectedHeadOID || strings.TrimSpace(result.ExpectedIndexSHA256) != result.ExpectedIndexSHA256 {
		return gitCommitArguments{}, false
	}
	return result, true
}
