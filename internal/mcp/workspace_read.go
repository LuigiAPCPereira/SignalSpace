package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// WorkspaceTextReader é uma porta estrita: somente a composição local pode
// conceder ou revogar o workspace. A ferramenta nunca recebe a raiz absoluta.
type WorkspaceTextReader interface {
	ReadText(owner, clientID, sessionID, relative string) (string, error)
}

type readToolAccess struct {
	reader    WorkspaceTextReader
	verify    func(context.Context) (VerifiedIdentity, error)
	challenge string
}

func readToolDefinition() map[string]any {
	return map[string]any{
		"name":        readToolName,
		"description": "Read UTF-8 text (up to 32 KiB) from an explicitly approved workspace. Provide the active session ID and a relative path; no edits or commands.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
			},
			"required":             []string{"session_id", "path"},
			"additionalProperties": false,
		},
		"securitySchemes": []any{map[string]any{"type": "oauth2", "scopes": []string{workspaceReadScope}}},
		"annotations":     map[string]any{"readOnlyHint": true, "destructiveHint": false},
	}
}

func (a *readToolAccess) call(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	// Falha de autenticação nunca encaminha argumentos ao armazenamento local.
	identity, err := a.verify(ctx)
	if err != nil {
		challenge := a.challenge + `, error="invalid_token", error_description="Invalid access token"`
		if errors.Is(err, ErrInsufficientScope) {
			challenge = a.challenge + `, error="insufficient_scope", error_description="Workspace read scope is required"`
		}
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Workspace read authorization required."}},
			"_meta":   map[string]any{"mcp/www_authenticate": []string{challenge}},
			"isError": true,
		}})
		return
	}
	var arguments map[string]json.RawMessage
	if json.Unmarshal(raw, &arguments) != nil || len(arguments) != 2 {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	var sessionID, relative string
	if json.Unmarshal(arguments["session_id"], &sessionID) != nil ||
		json.Unmarshal(arguments["path"], &relative) != nil || sessionID == "" || relative == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	text, err := a.reader.ReadText(identity.OwnerSubject, identity.ClientID, sessionID, relative)
	if err != nil {
		// Não revelar paths, detalhes do filesystem ou presença de arquivo a
		// uma solicitação recusada; somente o terminal conhece a raiz.
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Workspace read unavailable or not authorized."}},
			"isError": true,
		}})
		return
	}
	reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []any{map[string]any{"type": "text", "text": text}},
		"isError": false,
	}})
}
