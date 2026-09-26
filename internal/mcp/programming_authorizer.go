package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/LuigiAPCPereira/SignalSpace/internal/approval"
	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

const programmingScope = "signalspace:programming"

var (
	ErrProgrammingAuthorizationDenied = errors.New("programming local authorization denied")
	ErrProgrammingApprovalRequired    = errors.New("programming local approval required")
	ErrProgrammingApprovalUnavailable = errors.New("programming local approval unavailable")
)

// ProgrammingAuthorizationInput é a fronteira explícita entre transporte e
// decisão local. O transporte entrega somente identidade já verificada e
// argumentos tipados/normalizados; não conhece Store, Engine ou Manager.
type ProgrammingAuthorizationInput struct {
	Identity    VerifiedIdentity
	Tool        string
	Capability  capability.Capability
	SessionID   string
	Fingerprint string
	Operation   any
	SafeSummary string
}

// ProgrammingAuthorizer decide a chamada antes do primeiro efeito local.
// Implementações owner-side podem revalidar grant, policy e permit sem que o
// handler MCP dependa da implementação concreta desses domínios.
type ProgrammingAuthorizer interface {
	Authorize(context.Context, ProgrammingAuthorizationInput) error
}

// ProgrammingAuthorizationError carrega somente o envelope público seguro de
// uma decisão local. Nunca contém bearer, conteúdo, raiz absoluta ou permit.
type ProgrammingAuthorizationError struct {
	Code       string
	RequestID  string
	Capability capability.Capability
	Summary    string
}

func (e *ProgrammingAuthorizationError) Error() string {
	if e == nil {
		return "programming authorization failed"
	}
	return e.Code
}

func programmingAuthorizationError(code string, input ProgrammingAuthorizationInput, requestID string) error {
	return &ProgrammingAuthorizationError{Code: code, RequestID: requestID, Capability: input.Capability, Summary: input.SafeSummary}
}

type programmingOperation struct {
	Capability  capability.Capability
	SessionID   string
	Fingerprint string
	Operation   any
	Summary     string
}

// normalizeProgrammingOperation valida a forma tipada antes de pedir
// aprovação. Os handlers de cada ferramenta repetem a validação completa
// antes de executar, portanto esta função nunca substitui precondições do
// filesystem/Git.
func normalizeProgrammingOperation(tool string, raw json.RawMessage) (programmingOperation, error) {
	var arguments map[string]json.RawMessage
	if json.Unmarshal(raw, &arguments) != nil || arguments == nil {
		return programmingOperation{}, errors.New("arguments must be an object")
	}
	if tool == toolName {
		return programmingOperation{}, errors.New("diagnostic has no local programming operation")
	}
	if !hasString(arguments, "session_id") {
		return programmingOperation{}, errors.New("session_id is required")
	}
	sessionID, _ := requiredString(arguments, "session_id")
	allowed := map[string]struct{}{"session_id": {}}
	required := map[string]struct{}{"session_id": {}}
	capabilityForTool := map[string]capability.Capability{
		readToolName: capability.WorkspaceRead, listDirectoryToolName: capability.WorkspaceRead,
		statPathToolName: capability.WorkspaceRead, findPathsToolName: capability.WorkspaceRead,
		searchTextToolName: capability.WorkspaceRead,
		writeToolName:      capability.WorkspaceWrite, createDirectoryToolName: capability.WorkspaceWrite,
		createTextFileToolName: capability.WorkspaceWrite, writeTextFileToolName: capability.WorkspaceWrite,
		copyPathToolName: capability.WorkspaceWrite, movePathToolName: capability.WorkspaceWrite,
		applyPatchToolName: capability.WorkspaceWrite,
		deleteFileToolName: capability.WorkspaceDelete, deleteDirectoryToolName: capability.WorkspaceDelete,
		gitReviewToolName: capability.GitReview, gitStatusToolName: capability.GitReview,
		stageGitPathsName: capability.GitIndex, unstageGitPathsName: capability.GitIndex,
		commitGitIndexToolName: capability.GitCommit,
	}
	cap, ok := capabilityForTool[tool]
	if !ok {
		return programmingOperation{}, errors.New("unknown programming tool")
	}
	add := func(name string, isRequired bool) {
		allowed[name] = struct{}{}
		if isRequired {
			required[name] = struct{}{}
		}
	}
	switch tool {
	case readToolName, listDirectoryToolName, statPathToolName:
		add("path", true)
	case findPathsToolName:
		add("pattern", true)
		add("root", false)
		add("max_results", false)
		add("max_depth", false)
	case searchTextToolName:
		add("query", true)
		add("root", false)
		add("max_results", false)
	case writeToolName:
		add("path", true)
		add("expected", true)
		add("replacement", true)
	case createDirectoryToolName, deleteFileToolName, deleteDirectoryToolName:
		add("path", true)
	case createTextFileToolName:
		add("path", true)
		add("content", true)
	case writeTextFileToolName:
		add("path", true)
		add("expected_sha256", true)
		add("content", true)
	case copyPathToolName, movePathToolName:
		add("source", true)
		add("destination", true)
	case applyPatchToolName:
		add("operations", true)
	case gitReviewToolName, gitStatusToolName:
	case stageGitPathsName, unstageGitPathsName:
		add("expected_index_sha256", true)
		add("entries", true)
	case commitGitIndexToolName:
		add("expected_head_oid", true)
		add("expected_index_sha256", true)
		add("message", true)
	}
	if len(arguments) != len(allowed) {
		return programmingOperation{}, errors.New("invalid argument set")
	}
	for name := range arguments {
		if _, exists := allowed[name]; !exists {
			return programmingOperation{}, errors.New("unknown argument")
		}
		if _, exists := required[name]; exists && !hasValue(arguments, name) {
			return programmingOperation{}, errors.New("required argument is invalid")
		}
	}
	if err := validateProgrammingArgumentTypes(arguments, tool); err != nil {
		return programmingOperation{}, err
	}
	if tool == applyPatchToolName {
		if _, err := parsePatchOperations(arguments["operations"]); err != nil {
			return programmingOperation{}, err
		}
	}
	if tool == stageGitPathsName || tool == unstageGitPathsName {
		if _, _, _, ok := decodeGitIndexArguments(raw); !ok {
			return programmingOperation{}, errors.New("invalid Git index arguments")
		}
	}
	if tool == commitGitIndexToolName {
		if _, ok := decodeGitCommitArguments(raw); !ok {
			return programmingOperation{}, errors.New("invalid Git commit arguments")
		}
	}
	var operation any
	if err := json.Unmarshal(raw, &operation); err != nil {
		return programmingOperation{}, errors.New("invalid operation")
	}
	operation = redactOperation(operation)
	fingerprint, err := approval.CanonicalFingerprint(tool, sessionID, operation)
	if err != nil {
		return programmingOperation{}, err
	}
	return programmingOperation{Capability: cap, SessionID: sessionID, Fingerprint: fingerprint, Operation: operation, Summary: programmingSummary(tool, operation)}, nil
}

func validateProgrammingArgumentTypes(arguments map[string]json.RawMessage, tool string) error {
	stringFields := map[string]struct{}{
		"session_id": {}, "path": {}, "root": {}, "pattern": {}, "query": {}, "source": {}, "destination": {},
		"expected": {}, "replacement": {}, "expected_sha256": {}, "expected_index_sha256": {}, "expected_head_oid": {}, "message": {}, "content": {},
	}
	for name, raw := range arguments {
		if _, ok := stringFields[name]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return errors.New("argument must be a string")
			}
			switch name {
			case "path", "root", "source", "destination", "pattern", "query":
				if len(value) > workspace.MaxRelativePathBytes {
					return errors.New("path argument is too long")
				}
			case "content", "replacement":
				if len(value) > workspace.MaxTextBytes {
					return errors.New("text argument is too large")
				}
			case "message":
				if len(value) > workspace.MaxGitCommitMessageBytes {
					return errors.New("commit message is too large")
				}
			case "expected_sha256", "expected_index_sha256":
				if len(value) != 64 || strings.TrimSpace(value) != value {
					return errors.New("invalid expected hash")
				}
				if _, err := hex.DecodeString(value); err != nil {
					return errors.New("invalid expected hash")
				}
			case "expected_head_oid":
				if strings.TrimSpace(value) != value || value == "" {
					return errors.New("invalid expected head")
				}
			}
			continue
		}
		if name == "max_results" || name == "max_depth" {
			var number json.Number
			if json.Unmarshal(raw, &number) != nil {
				return errors.New("numeric argument is invalid")
			}
			value, err := strconv.Atoi(number.String())
			if err != nil || value < 1 {
				return errors.New("numeric argument is invalid")
			}
			if name == "max_results" && (tool == findPathsToolName && value > workspace.MaxFindResults || tool == searchTextToolName && value > workspace.MaxSearchMatches) {
				return errors.New("result limit is invalid")
			}
			if name == "max_depth" && value > workspace.MaxFindDepth {
				return errors.New("depth limit is invalid")
			}
		}
	}
	return nil
}

func hasString(arguments map[string]json.RawMessage, name string) bool {
	value, ok := requiredString(arguments, name)
	return ok && value != ""
}

func hasValue(arguments map[string]json.RawMessage, name string) bool {
	if name == "session_id" || name == "path" || name == "source" || name == "destination" || name == "pattern" || name == "query" || name == "expected" || name == "replacement" || name == "expected_sha256" || name == "expected_head_oid" || name == "message" || name == "content" {
		return hasString(arguments, name)
	}
	return len(arguments[name]) > 0 && string(arguments[name]) != "null"
}

func redactOperation(value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, nested := range item {
			if key == "content" || key == "replacement" {
				encoded := []byte(fmt.Sprint(nested))
				if text, ok := nested.(string); ok {
					encoded = []byte(text)
				}
				digest := sha256.Sum256(encoded)
				result[key+"_sha256"] = hex.EncodeToString(digest[:])
				continue
			}
			result[key] = redactOperation(nested)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for index, nested := range item {
			result[index] = redactOperation(nested)
		}
		return result
	default:
		return value
	}
}

func programmingSummary(tool string, operation any) string {
	values := make([]string, 0, 2)
	if object, ok := operation.(map[string]any); ok {
		for _, key := range []string{"path", "source", "destination"} {
			if value, ok := object[key].(string); ok && value != "" {
				values = append(values, value)
			}
		}
	}
	text := tool
	if len(values) > 0 {
		text += " " + strings.Join(values, " -> ")
	}
	var builder strings.Builder
	for _, r := range text {
		if unicode.IsControl(r) {
			builder.WriteRune(' ')
		} else {
			builder.WriteRune(r)
		}
		if builder.Len() >= 220 {
			break
		}
	}
	return strings.TrimSpace(builder.String())
}

func programmingToolDescription(tool string) string {
	descriptions := map[string]string{
		readToolName:            "Read bounded UTF-8 text from an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization; no edits or commands.",
		listDirectoryToolName:   "List bounded entries in an approved workspace directory. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		statPathToolName:        "Return bounded metadata for a relative path in an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		findPathsToolName:       "Find bounded paths in an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		searchTextToolName:      "Search bounded literal text in an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		writeToolName:           "Replace an exact UTF-8 text value in an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		createDirectoryToolName: "Create one relative directory in an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		createTextFileToolName:  "Create one new UTF-8 text file in an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		writeTextFileToolName:   "Update one existing UTF-8 text file with a hash precondition. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		copyPathToolName:        "Copy bounded regular files or directories inside an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		movePathToolName:        "Move one bounded path inside an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		deleteFileToolName:      "Delete one regular file in an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		deleteDirectoryToolName: "Delete one empty directory in an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		applyPatchToolName:      "Apply a bounded structured patch inside an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		gitReviewToolName:       "Review Git status and diff for an approved workspace without staging, committing or pushing. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		gitStatusToolName:       "Return bounded local Git status for an approved workspace. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		stageGitPathsName:       "Stage explicitly listed paths in an approved managed worktree. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		unstageGitPathsName:     "Unstage explicitly listed paths in an approved managed worktree. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
		commitGitIndexToolName:  "Create one staged-only local commit in an approved managed worktree. Requires an authenticated SignalSpace Programming connection and local SignalSpace authorization.",
	}
	return descriptions[tool]
}

func programmingScopeChallenge(metadataURL string) string {
	return fmt.Sprintf(`Bearer resource_metadata="%s", scope="%s"`, metadataURL, programmingScope)
}

func writeProgrammingAuthorizationError(w http.ResponseWriter, id any, err error) {
	code := "LOCAL_APPROVAL_UNAVAILABLE"
	requestID := ""
	capabilityName := ""
	summary := ""
	if typed, ok := err.(*ProgrammingAuthorizationError); ok && typed != nil {
		code, requestID, capabilityName, summary = typed.Code, typed.RequestID, string(typed.Capability), typed.Summary
	}
	if code == "LOCAL_AUTHORIZATION_DENIED" {
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Local SignalSpace authorization denied."}},
			"isError": true,
		}})
		return
	}
	if code == "LOCAL_APPROVAL_REQUIRED" {
		structured := map[string]any{
			"code":                     code,
			"request_id":               requestID,
			"capability":               capabilityName,
			"status":                   "pending",
			"retryable_after_approval": true,
			"summary":                  summary,
		}
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content":           []any{map[string]any{"type": "text", "text": "SignalSpace requires local approval. Approve the pending request in the local SignalSpace panel, then retry the same operation."}},
			"structuredContent": structured,
			"isError":           true,
		}})
		return
	}
	reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content":           []any{map[string]any{"type": "text", "text": "Local SignalSpace approval is unavailable; operation was not executed."}},
		"structuredContent": map[string]any{"code": "LOCAL_APPROVAL_UNAVAILABLE", "status": "unavailable", "retryable_after_approval": false},
		"isError":           true,
	}})
}
