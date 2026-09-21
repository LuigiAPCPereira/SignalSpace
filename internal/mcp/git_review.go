package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/LuigiAPCPereira/SignalSpace/internal/programming"
)

const (
	gitReviewScope    = "signalspace:git.review"
	gitReviewToolName = "review_git_changes"
)

// WorkspaceGitReviewer é uma porta estrita para a composição MCP de teste.
// A implementação deve revalidar owner, cliente, sessão e concessão atual;
// nenhuma raiz ou Session é fornecida pelo transporte.
type WorkspaceGitReviewer interface {
	ReviewGit(context.Context, string, string, string) (programming.DiffReview, error)
}

type gitToolAccess struct {
	reviewer  WorkspaceGitReviewer
	verify    func(context.Context) (VerifiedIdentity, error)
	challenge string
	advertise bool
}

func gitReviewToolDefinition() map[string]any {
	return map[string]any{
		"name":        gitReviewToolName,
		"description": "Review the approved workspace Git status and diff against a local baseline. Read-only: never stages, commits, pushes or executes arbitrary commands.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"session_id": map[string]any{"type": "string"}},
			"required":             []string{"session_id"},
			"additionalProperties": false,
		},
		"securitySchemes": []any{map[string]any{"type": "oauth2", "scopes": []string{gitReviewScope}}},
		"annotations":     map[string]any{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true},
	}
}

func (a *gitToolAccess) call(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, err := a.verify(ctx)
	if err != nil {
		challenge := a.challenge + `, error="invalid_token", error_description="Invalid access token"`
		if errors.Is(err, ErrInsufficientScope) {
			challenge = a.challenge + `, error="insufficient_scope", error_description="Git review scope is required"`
		}
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Git review authorization required."}},
			"_meta":   map[string]any{"mcp/www_authenticate": []string{challenge}},
			"isError": true,
		}})
		return
	}
	var arguments map[string]json.RawMessage
	if json.Unmarshal(raw, &arguments) != nil || len(arguments) != 1 {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	sessionID, ok := requiredString(arguments, "session_id")
	if !ok || sessionID == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	review, err := a.reviewer.ReviewGit(ctx, identity.OwnerSubject, identity.ClientID, sessionID)
	if err != nil {
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Git review unavailable or not authorized."}},
			"isError": true,
		}})
		return
	}
	payload, err := json.Marshal(struct {
		Before       gitReviewSnapshot `json:"before"`
		After        gitReviewSnapshot `json:"after"`
		Complete     bool              `json:"complete"`
		StatusChange bool              `json:"status_changed"`
		DiffChange   bool              `json:"diff_changed"`
	}{
		Before:       gitReviewSnapshot{Status: review.Before.Status, Diff: review.Before.Diff, OutputTruncated: review.Before.OutputTruncated},
		After:        gitReviewSnapshot{Status: review.After.Status, Diff: review.After.Diff, OutputTruncated: review.After.OutputTruncated},
		Complete:     review.Complete,
		StatusChange: review.StatusChange,
		DiffChange:   review.DiffChange,
	})
	if err != nil {
		fail(w, http.StatusOK, id, -32603, "Git review unavailable")
		return
	}
	reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(payload)}},
		"isError": false,
	}})
}

type gitReviewSnapshot struct {
	Status          string `json:"status"`
	Diff            string `json:"diff"`
	OutputTruncated bool   `json:"output_truncated"`
}
