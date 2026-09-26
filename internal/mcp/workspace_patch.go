package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

type WorkspacePatchApplier interface {
	ApplyPatch(owner, clientID, sessionID string, operations []workspace.PatchOperation) (workspace.PatchResult, error)
}

func applyPatchToolDefinition() map[string]any {
	operationSchema := func(properties map[string]any, required []string) map[string]any {
		return map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             required,
			"additionalProperties": false,
		}
	}
	stringProperty := func(maxLength int) map[string]any {
		property := map[string]any{"type": "string"}
		if maxLength > 0 {
			property["maxLength"] = maxLength
		}
		return property
	}
	operations := []any{
		operationSchema(map[string]any{
			"type":    map[string]any{"const": workspace.PatchCreateFile},
			"path":    stringProperty(workspace.MaxRelativePathBytes),
			"content": stringProperty(workspace.MaxTextBytes),
		}, []string{"type", "path", "content"}),
		operationSchema(map[string]any{
			"type":            map[string]any{"const": workspace.PatchWriteFile},
			"path":            stringProperty(workspace.MaxRelativePathBytes),
			"expected_sha256": stringProperty(64),
			"content":         stringProperty(workspace.MaxTextBytes),
		}, []string{"type", "path", "expected_sha256", "content"}),
		operationSchema(map[string]any{
			"type":        map[string]any{"const": workspace.PatchMovePath},
			"source":      stringProperty(workspace.MaxRelativePathBytes),
			"destination": stringProperty(workspace.MaxRelativePathBytes),
		}, []string{"type", "source", "destination"}),
		operationSchema(map[string]any{
			"type":            map[string]any{"const": workspace.PatchDeleteFile},
			"path":            stringProperty(workspace.MaxRelativePathBytes),
			"expected_sha256": stringProperty(64),
		}, []string{"type", "path", "expected_sha256"}),
		operationSchema(map[string]any{
			"type": map[string]any{"const": workspace.PatchCreateDirectory},
			"path": stringProperty(workspace.MaxRelativePathBytes),
		}, []string{"type", "path"}),
	}
	return map[string]any{
		"name":        applyPatchToolName,
		"description": "Apply a bounded structured multi-file patch inside the approved workspace. Operations are preflighted with closed schemas, create/update/delete hashes and descriptor-relative path checks; no arbitrary diff, shell, Git mutation or recursive delete is available.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"operations": map[string]any{"type": "array", "minItems": 1, "maxItems": workspace.MaxPatchOperations, "items": map[string]any{"oneOf": operations}},
			},
			"required":             []string{"session_id", "operations"},
			"additionalProperties": false,
		},
		"securitySchemes": oauthSecuritySchemes(workspaceWriteScope),
		"annotations":     map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": false},
	}
}

func (a *writeToolAccess) applyPatch(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, arguments, ok := a.authorizeWriteArguments(w, ctx, id, raw, 2)
	if !ok {
		return
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	if !sessionOK || sessionID == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	operationsRaw, operationsOK := arguments["operations"]
	if !operationsOK {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	operations, err := parsePatchOperations(operationsRaw)
	if err != nil {
		result := workspace.PatchResult{Status: "invalid_patch", Operations: []workspace.PatchOperationResult{}, Rollback: "not_started"}
		replyStructuredErrorWithMessage(w, id, "Structured workspace patch was rejected before mutation.", result)
		return
	}
	result, err := a.patchApplier.ApplyPatch(identity.OwnerSubject, identity.ClientID, sessionID, operations)
	if err != nil {
		if result.Status == "" {
			result.Status = patchAuthorizationStatus(err)
			result.Rollback = "not_started"
		}
		replyStructuredErrorWithMessage(w, id, "Structured workspace patch failed or was conflicted; inspect the bounded result.", result)
		return
	}
	replyStructured(w, id, result, false)
}

func patchAuthorizationStatus(err error) string {
	if errors.Is(err, workspace.ErrNotAuthorized) {
		return "unauthorized"
	}
	if errors.Is(err, workspace.ErrClosed) {
		return "unknown"
	}
	return "internal"
}

func parsePatchOperations(raw json.RawMessage) ([]workspace.PatchOperation, error) {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil || len(values) == 0 || len(values) > workspace.MaxPatchOperations {
		return nil, errors.New("invalid patch operations")
	}
	operations := make([]workspace.PatchOperation, 0, len(values))
	for _, value := range values {
		var fields map[string]json.RawMessage
		if json.Unmarshal(value, &fields) != nil || fields == nil {
			return nil, errors.New("invalid patch operation")
		}
		typeValue, ok := requiredString(fields, "type")
		if !ok {
			return nil, errors.New("patch operation type is required")
		}
		allowed := map[string]struct{}{}
		switch typeValue {
		case workspace.PatchCreateFile:
			allowed = map[string]struct{}{"type": {}, "path": {}, "content": {}}
		case workspace.PatchWriteFile:
			allowed = map[string]struct{}{"type": {}, "path": {}, "expected_sha256": {}, "content": {}}
		case workspace.PatchMovePath:
			allowed = map[string]struct{}{"type": {}, "source": {}, "destination": {}}
		case workspace.PatchDeleteFile:
			allowed = map[string]struct{}{"type": {}, "path": {}, "expected_sha256": {}}
		case workspace.PatchCreateDirectory:
			allowed = map[string]struct{}{"type": {}, "path": {}}
		default:
			return nil, errors.New("unknown patch operation")
		}
		if len(fields) != len(allowed) {
			return nil, errors.New("patch operation has extra or missing fields")
		}
		for name := range fields {
			if _, ok := allowed[name]; !ok {
				return nil, errors.New("patch operation has an unknown field")
			}
		}
		operation := workspace.PatchOperation{Type: typeValue}
		var fieldOK bool
		switch typeValue {
		case workspace.PatchCreateFile:
			operation.Path, fieldOK = requiredString(fields, "path")
			if !fieldOK {
				return nil, errors.New("patch path is required")
			}
			operation.Content, fieldOK = requiredString(fields, "content")
		case workspace.PatchWriteFile:
			operation.Path, fieldOK = requiredString(fields, "path")
			if !fieldOK {
				return nil, errors.New("patch path is required")
			}
			operation.ExpectedSHA256, fieldOK = requiredString(fields, "expected_sha256")
			if !fieldOK {
				return nil, errors.New("patch expected hash is required")
			}
			operation.Content, fieldOK = requiredString(fields, "content")
		case workspace.PatchMovePath:
			operation.Source, fieldOK = requiredString(fields, "source")
			if !fieldOK {
				return nil, errors.New("patch source is required")
			}
			operation.Destination, fieldOK = requiredString(fields, "destination")
		case workspace.PatchDeleteFile:
			operation.Path, fieldOK = requiredString(fields, "path")
			if !fieldOK {
				return nil, errors.New("patch path is required")
			}
			operation.ExpectedSHA256, fieldOK = requiredString(fields, "expected_sha256")
		case workspace.PatchCreateDirectory:
			operation.Path, fieldOK = requiredString(fields, "path")
		}
		if !fieldOK {
			return nil, errors.New("patch operation field is invalid")
		}
		operations = append(operations, operation)
	}
	return operations, nil
}
