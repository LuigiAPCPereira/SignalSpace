package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

const writeToolName = "replace_text"

// WorkspaceTextWriter é uma porta estrita para a composição MCP de teste.
// A implementação deve consultar a concessão atual em cada chamada.
type WorkspaceTextWriter interface {
	ReplaceText(owner, clientID, sessionID, relative, expected, replacement string) error
}

type writeToolAccess struct {
	writer    WorkspaceTextWriter
	verify    func(context.Context) (VerifiedIdentity, error)
	challenge string
	advertise bool
}

func writeToolSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session_id":  map[string]any{"type": "string"},
			"path":        map[string]any{"type": "string"},
			"expected":    map[string]any{"type": "string"},
			"replacement": map[string]any{"type": "string"},
		},
		"required":             []string{"session_id", "path", "expected", "replacement"},
		"additionalProperties": false,
	}
}

func writeToolDefinition() map[string]any {
	return map[string]any{
		"name":            writeToolName,
		"description":     "Replace an exact UTF-8 text value in an explicitly approved workspace. Requires a separate write scope and active local write grant; no commands or Git mutations.",
		"inputSchema":     writeToolSchema(),
		"securitySchemes": []any{map[string]any{"type": "oauth2", "scopes": []string{workspaceWriteScope}}},
		"annotations":     map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": false},
	}
}

func (a *writeToolAccess) authorizeWorkspaceCall(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) (VerifiedIdentity, string, string, string, string, bool) {
	identity, err := a.verify(ctx)
	if err != nil {
		challenge := a.challenge + `, error="invalid_token", error_description="Invalid access token"`
		if errors.Is(err, ErrInsufficientScope) {
			challenge = a.challenge + `, error="insufficient_scope", error_description="Workspace write scope is required"`
		}
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Workspace write authorization required."}},
			"_meta":   map[string]any{"mcp/www_authenticate": []string{challenge}},
			"isError": true,
		}})
		return VerifiedIdentity{}, "", "", "", "", false
	}
	var arguments map[string]json.RawMessage
	if json.Unmarshal(raw, &arguments) != nil || len(arguments) != 4 {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return VerifiedIdentity{}, "", "", "", "", false
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	relative, pathOK := requiredString(arguments, "path")
	expected, expectedOK := requiredString(arguments, "expected")
	replacement, replacementOK := requiredString(arguments, "replacement")
	if !sessionOK || !pathOK || !expectedOK || !replacementOK || sessionID == "" || relative == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return VerifiedIdentity{}, "", "", "", "", false
	}
	return identity, sessionID, relative, expected, replacement, true
}

func requiredString(arguments map[string]json.RawMessage, name string) (string, bool) {
	raw, ok := arguments[name]
	if !ok || string(raw) == "null" {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func (a *writeToolAccess) call(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, sessionID, relative, expected, replacement, ok := a.authorizeWorkspaceCall(w, ctx, id, raw)
	if !ok {
		return
	}
	if err := a.writer.ReplaceText(identity.OwnerSubject, identity.ClientID, sessionID, relative, expected, replacement); err != nil {
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Workspace write unavailable or not authorized."}},
			"isError": true,
		}})
		return
	}
	reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []any{map[string]any{"type": "text", "text": "Workspace text replaced."}},
		"isError": false,
	}})
}
