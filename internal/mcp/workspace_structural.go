package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

type WorkspaceCopier interface {
	Copy(owner, clientID, sessionID, source, destination string) (workspace.CopyResult, error)
}

type WorkspaceMover interface {
	Move(owner, clientID, sessionID, source, destination string) (workspace.MoveResult, error)
}

type WorkspaceFileDeleter interface {
	DeleteFile(owner, clientID, sessionID, relative string) (workspace.DeleteResult, error)
}

type WorkspaceDirectoryDeleter interface {
	DeleteDirectory(owner, clientID, sessionID, relative string) (workspace.DeleteResult, error)
}

func copyPathToolDefinition() map[string]any {
	return map[string]any{
		"name":            copyPathToolName,
		"description":     "Copy one regular file or a tree of regular files and directories inside the approved workspace. The destination must be absent; symlinks and special files are rejected, entries/depth/bytes are bounded, files are created as owner-only 0600 and partial cleanup is reported.",
		"inputSchema":     structuralPairSchema(),
		"securitySchemes": oauthSecuritySchemes(workspaceWriteScope),
		"annotations":     map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false},
	}
}

func movePathToolDefinition() map[string]any {
	return map[string]any{
		"name":            movePathToolName,
		"description":     "Move one regular file or directory inside the approved workspace with descriptor-relative no-replace rename. The destination must be absent, directories cannot move into themselves, and cross-device moves are reported unsupported rather than copied and deleted.",
		"inputSchema":     structuralPairSchema(),
		"securitySchemes": oauthSecuritySchemes(workspaceWriteScope),
		"annotations":     map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": false},
	}
}

func deleteFileToolDefinition() map[string]any {
	return structuralDeleteDefinition(deleteFileToolName, "Delete exactly one regular file in the approved workspace. Directories, symlinks and special files are rejected; the operation never recurses.")
}

func deleteDirectoryToolDefinition() map[string]any {
	return structuralDeleteDefinition(deleteDirectoryToolName, "Delete one empty directory in the approved workspace. The session root, non-empty directories, symlinks and recursive deletion are rejected.")
}

func structuralPairSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session_id":  map[string]any{"type": "string"},
			"source":      map[string]any{"type": "string"},
			"destination": map[string]any{"type": "string"},
		},
		"required":             []string{"session_id", "source", "destination"},
		"additionalProperties": false,
	}
}

func structuralDeleteDefinition(name, description string) map[string]any {
	return map[string]any{
		"name":            name,
		"description":     description,
		"inputSchema":     workspaceToolSchema(),
		"securitySchemes": oauthSecuritySchemes(workspaceWriteScope),
		"annotations":     map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": false},
	}
}

func (a *writeToolAccess) copyPath(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, arguments, ok := a.authorizeWriteArguments(w, ctx, id, raw, 3)
	if !ok {
		return
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	source, sourceOK := requiredString(arguments, "source")
	destination, destinationOK := requiredString(arguments, "destination")
	if !sessionOK || !sourceOK || !destinationOK || sessionID == "" || source == "" || destination == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	result, err := a.copier.Copy(identity.OwnerSubject, identity.ClientID, sessionID, source, destination)
	if err != nil {
		replyStructuralError(w, id, "Workspace copy failed or is not authorized.", result, err)
		return
	}
	replyStructured(w, id, result, false)
}

func (a *writeToolAccess) movePath(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, arguments, ok := a.authorizeWriteArguments(w, ctx, id, raw, 3)
	if !ok {
		return
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	source, sourceOK := requiredString(arguments, "source")
	destination, destinationOK := requiredString(arguments, "destination")
	if !sessionOK || !sourceOK || !destinationOK || sessionID == "" || source == "" || destination == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	result, err := a.mover.Move(identity.OwnerSubject, identity.ClientID, sessionID, source, destination)
	if err != nil {
		replyStructuralError(w, id, "Workspace move failed or is not authorized.", result, err)
		return
	}
	replyStructured(w, id, result, false)
}

func (a *writeToolAccess) deleteFile(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, sessionID, relative, ok := a.authorizePathCall(w, ctx, id, raw)
	if !ok {
		return
	}
	result, err := a.fileDeleter.DeleteFile(identity.OwnerSubject, identity.ClientID, sessionID, relative)
	if err != nil {
		replyStructuralError(w, id, "Workspace file deletion failed or is not authorized.", result, err)
		return
	}
	replyStructured(w, id, result, false)
}

func (a *writeToolAccess) deleteDirectory(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, sessionID, relative, ok := a.authorizePathCall(w, ctx, id, raw)
	if !ok {
		return
	}
	result, err := a.directoryDeleter.DeleteDirectory(identity.OwnerSubject, identity.ClientID, sessionID, relative)
	if err != nil {
		replyStructuralError(w, id, "Workspace directory deletion failed or is not authorized.", result, err)
		return
	}
	replyStructured(w, id, result, false)
}

func structuralErrorStatus(err error) string {
	switch {
	case errors.Is(err, workspace.ErrReservedPath):
		return "reserved_path"
	case errors.Is(err, workspace.ErrPathExists):
		return "already_exists"
	case errors.Is(err, workspace.ErrInvalidPath), errors.Is(err, workspace.ErrUnsafePath), errors.Is(err, workspace.ErrUnsupportedType), errors.Is(err, workspace.ErrNotFile):
		return "invalid_input"
	case errors.Is(err, os.ErrNotExist):
		return "absent"
	case errors.Is(err, workspace.ErrDirectoryNotEmpty):
		return "not_empty"
	case errors.Is(err, workspace.ErrCrossDevice):
		return "cross_device_unsupported"
	case errors.Is(err, workspace.ErrStructuralLimit):
		return "limit_exceeded"
	case errors.Is(err, workspace.ErrOperationUnknown):
		return "unknown"
	default:
		return "unavailable_or_not_authorized"
	}
}

func replyStructuralError(w http.ResponseWriter, id any, message string, payload any, err error) {
	if result, ok := payload.(workspace.CopyResult); ok && result.Status == "" {
		result.Status = structuralErrorStatus(err)
		payload = result
	}
	if result, ok := payload.(workspace.MoveResult); ok && result.Status == "" {
		result.Status = structuralErrorStatus(err)
		payload = result
	}
	if result, ok := payload.(workspace.DeleteResult); ok && result.Status == "" {
		result.Status = structuralErrorStatus(err)
		payload = result
	}
	replyStructuredErrorWithMessage(w, id, message, payload)
}
