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

// WorkspaceDirectoryLister é uma capacidade opcional, injetada separadamente.
// A implementação deve aplicar a concessão atual em cada chamada.
type WorkspaceDirectoryLister interface {
	ListDirectory(owner, clientID, sessionID, relative string) ([]string, error)
}

type readToolAccess struct {
	reader    WorkspaceTextReader
	lister    WorkspaceDirectoryLister
	verify    func(context.Context) (VerifiedIdentity, error)
	challenge string
}

func workspaceToolSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session_id": map[string]any{"type": "string"},
			"path":       map[string]any{"type": "string"},
		},
		"required":             []string{"session_id", "path"},
		"additionalProperties": false,
	}
}

func readToolDefinition() map[string]any {
	return map[string]any{
		"name":            readToolName,
		"description":     "Read UTF-8 text (up to 32 KiB) from an explicitly approved workspace. Provide the active session ID and a relative path; no edits or commands.",
		"inputSchema":     workspaceToolSchema(),
		"securitySchemes": []any{map[string]any{"type": "oauth2", "scopes": []string{workspaceReadScope}}},
		"annotations":     map[string]any{"readOnlyHint": true, "destructiveHint": false},
	}
}

func listDirectoryToolDefinition() map[string]any {
	return map[string]any{
		"name":            listDirectoryToolName,
		"description":     "List up to 128 UTF-8 entry names in an approved directory. Provide the active session ID and a relative directory path, or '.' for the approved root. No file contents, types or absolute paths are returned.",
		"inputSchema":     workspaceToolSchema(),
		"securitySchemes": []any{map[string]any{"type": "oauth2", "scopes": []string{workspaceReadScope}}},
		"annotations":     map[string]any{"readOnlyHint": true, "destructiveHint": false},
	}
}

// authorizeWorkspaceCall autentica antes de interpretar argumentos e reaproveita
// exatamente a mesma fronteira de identidade nas duas ferramentas.
func (a *readToolAccess) authorizeWorkspaceCall(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) (VerifiedIdentity, string, string, bool) {
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
		return VerifiedIdentity{}, "", "", false
	}
	var arguments map[string]json.RawMessage
	if json.Unmarshal(raw, &arguments) != nil || len(arguments) != 2 {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return VerifiedIdentity{}, "", "", false
	}
	var sessionID, relative string
	if json.Unmarshal(arguments["session_id"], &sessionID) != nil ||
		json.Unmarshal(arguments["path"], &relative) != nil || sessionID == "" || relative == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return VerifiedIdentity{}, "", "", false
	}
	return identity, sessionID, relative, true
}

func (a *readToolAccess) call(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, sessionID, relative, ok := a.authorizeWorkspaceCall(w, ctx, id, raw)
	if !ok {
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

func (a *readToolAccess) list(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, sessionID, relative, ok := a.authorizeWorkspaceCall(w, ctx, id, raw)
	if !ok {
		return
	}
	names, err := a.lister.ListDirectory(identity.OwnerSubject, identity.ClientID, sessionID, relative)
	if err != nil {
		// Erros de autorização, path e filesystem são indistinguíveis no MCP.
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Workspace directory listing unavailable or not authorized."}},
			"isError": true,
		}})
		return
	}
	if names == nil {
		names = []string{}
	}
	payload, err := json.Marshal(struct {
		Entries []string `json:"entries"`
	}{Entries: names})
	if err != nil {
		fail(w, http.StatusOK, id, -32603, "Directory listing unavailable")
		return
	}
	reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(payload)}},
		"isError": false,
	}})
}
