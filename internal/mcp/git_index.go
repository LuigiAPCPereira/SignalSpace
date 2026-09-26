package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

const (
	gitIndexScope       = workspace.ScopeGitIndex
	stageGitPathsName   = "stage_git_paths"
	unstageGitPathsName = "unstage_git_paths"
)

// WorkspaceGitIndexMutator é uma porta isolada para o índice. Não fornece
// shell, commit, branch, remoto ou qualquer porta de filesystem ao transporte.
type WorkspaceGitIndexMutator interface {
	StageGitPaths(context.Context, string, string, string, string, []workspace.GitIndexEntry) (workspace.GitIndexMutationResult, error)
	UnstageGitPaths(context.Context, string, string, string, string, []workspace.GitIndexEntry) (workspace.GitIndexMutationResult, error)
}

type gitIndexToolAccess struct {
	mutator      WorkspaceGitIndexMutator
	verify       func(context.Context) (VerifiedIdentity, error)
	challenge    string
	discoverable bool
}

func stageGitPathsDefinition() map[string]any {
	return gitIndexToolDefinition(stageGitPathsName, "Stage explicitly listed local Git paths after index and working-file preconditions. Never stages all paths and never commits or pushes.", false)
}

func unstageGitPathsDefinition() map[string]any {
	return gitIndexToolDefinition(unstageGitPathsName, "Unstage explicitly listed local Git paths while retaining working-tree bytes. Never resets, cleans, commits or pushes.", true)
}

func gitIndexToolDefinition(name, description string, readOnly bool) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":            map[string]any{"type": "string"},
				"expected_index_sha256": map[string]any{"type": "string", "minLength": 64, "maxLength": 64},
				"entries":               map[string]any{"type": "array", "minItems": 1, "maxItems": workspace.MaxGitIndexPaths, "items": map[string]any{"type": "object"}},
			},
			"required":             []string{"session_id", "expected_index_sha256", "entries"},
			"additionalProperties": false,
		},
		"securitySchemes": oauthSecuritySchemes(gitIndexScope),
		"annotations":     map[string]any{"readOnlyHint": readOnly, "destructiveHint": !readOnly, "idempotentHint": false},
	}
}

func (a *gitIndexToolAccess) call(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage, stage bool) {
	identity, err := a.verify(ctx)
	if err != nil {
		challenge := a.challenge + `, error="invalid_token", error_description="Invalid access token"`
		if errors.Is(err, ErrInsufficientScope) {
			challenge = a.challenge + `, error="insufficient_scope", error_description="Git index scope is required"`
		}
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Git index authorization required."}},
			"_meta":   map[string]any{"mcp/www_authenticate": []string{challenge}},
			"isError": true,
		}})
		return
	}
	sessionID, expected, entries, ok := decodeGitIndexArguments(raw)
	if !ok {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	var result workspace.GitIndexMutationResult
	if stage {
		result, err = a.mutator.StageGitPaths(ctx, identity.OwnerSubject, identity.ClientID, sessionID, expected, entries)
	} else {
		result, err = a.mutator.UnstageGitPaths(ctx, identity.OwnerSubject, identity.ClientID, sessionID, expected, entries)
	}
	if err != nil {
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Git index operation unavailable or not authorized."}},
			"isError": true,
		}})
		return
	}
	payload, err := json.Marshal(result)
	if err != nil {
		fail(w, http.StatusOK, id, -32603, "Git index operation unavailable")
		return
	}
	reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(payload)}},
		"isError": false,
	}})
}

func decodeGitIndexArguments(raw json.RawMessage) (string, string, []workspace.GitIndexEntry, bool) {
	var arguments map[string]json.RawMessage
	if json.Unmarshal(raw, &arguments) != nil || len(arguments) != 3 {
		return "", "", nil, false
	}
	for key := range arguments {
		if key != "session_id" && key != "expected_index_sha256" && key != "entries" {
			return "", "", nil, false
		}
	}
	var sessionID, expected string
	if json.Unmarshal(arguments["session_id"], &sessionID) != nil || sessionID == "" || json.Unmarshal(arguments["expected_index_sha256"], &expected) != nil || strings.TrimSpace(expected) != expected {
		return "", "", nil, false
	}
	var rawEntries []json.RawMessage
	if json.Unmarshal(arguments["entries"], &rawEntries) != nil || len(rawEntries) == 0 || len(rawEntries) > workspace.MaxGitIndexPaths {
		return "", "", nil, false
	}
	entries := make([]workspace.GitIndexEntry, 0, len(rawEntries))
	for _, rawEntry := range rawEntries {
		var object map[string]json.RawMessage
		if json.Unmarshal(rawEntry, &object) != nil || object == nil || len(object) < 1 || len(object) > 3 {
			return "", "", nil, false
		}
		for key := range object {
			if key != "path" && key != "expected_sha256" && key != "expected_index_oid" {
				return "", "", nil, false
			}
		}
		var entry workspace.GitIndexEntry
		if json.Unmarshal(rawEntry, &entry) != nil || entry.Path == "" {
			return "", "", nil, false
		}
		entries = append(entries, entry)
	}
	return sessionID, expected, entries, true
}
