package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

const writeToolName = "replace_text"

// WorkspaceTextWriter é uma porta estrita para a composição MCP de teste.
// A implementação deve consultar a concessão atual em cada chamada.
type WorkspaceTextWriter interface {
	ReplaceText(owner, clientID, sessionID, relative, expected, replacement string) error
}

type WorkspaceDirectoryCreator interface {
	CreateDirectory(owner, clientID, sessionID, relative string) (workspace.DirectoryResult, error)
}

type WorkspaceTextCreator interface {
	CreateTextFile(owner, clientID, sessionID, relative, content string) (workspace.TextFileResult, error)
}

type WorkspaceTextUpdater interface {
	WriteTextFile(owner, clientID, sessionID, relative, expectedSHA256, content string) (workspace.TextFileResult, error)
}

type WorkspaceStructuralCopy interface {
	Copy(owner, clientID, sessionID, source, destination string) (workspace.CopyResult, error)
}

type WorkspaceStructuralMove interface {
	Move(owner, clientID, sessionID, source, destination string) (workspace.MoveResult, error)
}

type WorkspaceStructuralFileDelete interface {
	DeleteFile(owner, clientID, sessionID, relative string) (workspace.DeleteResult, error)
}

type WorkspaceStructuralDirectoryDelete interface {
	DeleteDirectory(owner, clientID, sessionID, relative string) (workspace.DeleteResult, error)
}

type writeToolAccess struct {
	writer           WorkspaceTextWriter
	directoryCreator WorkspaceDirectoryCreator
	textCreator      WorkspaceTextCreator
	textUpdater      WorkspaceTextUpdater
	copier           WorkspaceStructuralCopy
	mover            WorkspaceStructuralMove
	fileDeleter      WorkspaceStructuralFileDelete
	directoryDeleter WorkspaceStructuralDirectoryDelete
	patchApplier     WorkspacePatchApplier
	verify           func(context.Context) (VerifiedIdentity, error)
	challenge        string
	discoverable     bool
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
		"securitySchemes": oauthSecuritySchemes(workspaceWriteScope),
		"annotations":     map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": false},
	}
}

func createDirectoryToolDefinition() map[string]any {
	return map[string]any{
		"name":            createDirectoryToolName,
		"description":     "Create exactly one relative directory in an approved workspace. Parent directories must already exist; an existing directory is reported idempotently and no path is followed through a symlink.",
		"inputSchema":     workspaceToolSchema(),
		"securitySchemes": oauthSecuritySchemes(workspaceWriteScope),
		"annotations":     map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true},
	}
}

func createTextFileToolDefinition() map[string]any {
	return map[string]any{
		"name":        createTextFileToolName,
		"description": "Create one new UTF-8 text file in an approved workspace. This is CREATE-ONLY, does not create parent directories, rejects NUL and oversized content, and never overwrites an existing path.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string"},
				"content":    map[string]any{"type": "string", "maxLength": workspace.MaxTextBytes},
			},
			"required":             []string{"session_id", "path", "content"},
			"additionalProperties": false,
		},
		"securitySchemes": oauthSecuritySchemes(workspaceWriteScope),
		"annotations":     map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false},
	}
}

func writeTextFileToolDefinition() map[string]any {
	return map[string]any{
		"name":        writeTextFileToolName,
		"description": "Replace the complete UTF-8 content of an existing regular file only when expected_sha256 matches the observed content. The update is atomic and conflicts never overwrite silently.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":      map[string]any{"type": "string"},
				"path":            map[string]any{"type": "string"},
				"expected_sha256": map[string]any{"type": "string", "minLength": 64, "maxLength": 64},
				"content":         map[string]any{"type": "string", "maxLength": workspace.MaxTextBytes},
			},
			"required":             []string{"session_id", "path", "expected_sha256", "content"},
			"additionalProperties": false,
		},
		"securitySchemes": oauthSecuritySchemes(workspaceWriteScope),
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

func (a *writeToolAccess) createDirectory(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, sessionID, relative, ok := a.authorizePathCall(w, ctx, id, raw)
	if !ok {
		return
	}
	result, err := a.directoryCreator.CreateDirectory(identity.OwnerSubject, identity.ClientID, sessionID, relative)
	if err != nil {
		replyStructuredWriteError(w, id, "Workspace directory creation unavailable or not authorized.", err)
		return
	}
	replyStructured(w, id, result, false)
}

func (a *writeToolAccess) createTextFile(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, arguments, ok := a.authorizeWriteArguments(w, ctx, id, raw, 3)
	if !ok {
		return
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	relative, pathOK := requiredString(arguments, "path")
	content, contentOK := requiredString(arguments, "content")
	if !sessionOK || !pathOK || !contentOK || sessionID == "" || relative == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	result, err := a.textCreator.CreateTextFile(identity.OwnerSubject, identity.ClientID, sessionID, relative, content)
	if err != nil {
		replyStructuredWriteError(w, id, "Workspace text file creation unavailable or not authorized.", err)
		return
	}
	replyStructured(w, id, result, false)
}

func (a *writeToolAccess) writeTextFile(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, arguments, ok := a.authorizeWriteArguments(w, ctx, id, raw, 4)
	if !ok {
		return
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	relative, pathOK := requiredString(arguments, "path")
	expected, expectedOK := requiredString(arguments, "expected_sha256")
	content, contentOK := requiredString(arguments, "content")
	if !sessionOK || !pathOK || !expectedOK || !contentOK || sessionID == "" || relative == "" || expected == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	result, err := a.textUpdater.WriteTextFile(identity.OwnerSubject, identity.ClientID, sessionID, relative, expected, content)
	if err != nil {
		replyStructuredWriteError(w, id, "Workspace text update unavailable, not authorized or conflicted.", err)
		return
	}
	replyStructured(w, id, result, false)
}

func (a *writeToolAccess) authorizePathCall(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) (VerifiedIdentity, string, string, bool) {
	identity, arguments, ok := a.authorizeWriteArguments(w, ctx, id, raw, 2)
	if !ok {
		return VerifiedIdentity{}, "", "", false
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	relative, pathOK := requiredString(arguments, "path")
	if !sessionOK || !pathOK || sessionID == "" || relative == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return VerifiedIdentity{}, "", "", false
	}
	return identity, sessionID, relative, true
}

func (a *writeToolAccess) authorizeWriteArguments(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage, argumentCount int) (VerifiedIdentity, map[string]json.RawMessage, bool) {
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
		return VerifiedIdentity{}, nil, false
	}
	var arguments map[string]json.RawMessage
	if json.Unmarshal(raw, &arguments) != nil || arguments == nil || len(arguments) != argumentCount {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return VerifiedIdentity{}, nil, false
	}
	return identity, arguments, true
}

func replyStructuredWriteError(w http.ResponseWriter, id any, message string, err error) {
	status := "unavailable_or_not_authorized"
	switch {
	case errors.Is(err, workspace.ErrReservedPath):
		status = "reserved_path"
	case errors.Is(err, workspace.ErrConflict):
		status = "conflict"
	case errors.Is(err, workspace.ErrPathExists):
		status = "already_exists"
	case errors.Is(err, workspace.ErrInvalidPath), errors.Is(err, workspace.ErrInvalidHash), errors.Is(err, workspace.ErrInvalidLimit), errors.Is(err, workspace.ErrNotText), errors.Is(err, workspace.ErrTooLarge), errors.Is(err, workspace.ErrUnsafePath), errors.Is(err, workspace.ErrUnsupportedType), errors.Is(err, workspace.ErrNotFile):
		status = "invalid_input"
	case errors.Is(err, workspace.ErrDirectoryNotEmpty):
		status = "not_empty"
	case errors.Is(err, workspace.ErrCrossDevice):
		status = "cross_device_unsupported"
	case errors.Is(err, workspace.ErrStructuralLimit):
		status = "limit_exceeded"
	case errors.Is(err, workspace.ErrOperationUnknown):
		status = "unknown"
	}
	replyStructuredErrorWithMessage(w, id, message, map[string]any{"status": status})
}
