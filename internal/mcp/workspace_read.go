package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
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

type WorkspacePathStatter interface {
	StatPath(owner, clientID, sessionID, relative string) (workspace.PathStat, error)
}

type WorkspacePathFinder interface {
	FindPaths(owner, clientID, sessionID, root, pattern string, maxResults, maxDepth int) (workspace.FindResult, error)
}

type WorkspaceTextSearcher interface {
	SearchText(owner, clientID, sessionID, root, query string, maxResults int) (workspace.SearchResult, error)
}

type readToolAccess struct {
	reader       WorkspaceTextReader
	lister       WorkspaceDirectoryLister
	statter      WorkspacePathStatter
	finder       WorkspacePathFinder
	searcher     WorkspaceTextSearcher
	verify       func(context.Context) (VerifiedIdentity, error)
	challenge    string
	discoverable bool
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
		"securitySchemes": oauthSecuritySchemes(workspaceReadScope),
		"annotations":     map[string]any{"readOnlyHint": true, "destructiveHint": false},
	}
}

func listDirectoryToolDefinition() map[string]any {
	return map[string]any{
		"name":            listDirectoryToolName,
		"description":     "List up to 128 UTF-8 entry names in an approved directory. Provide the active session ID and a relative directory path, or '.' for the approved root. No file contents, types or absolute paths are returned.",
		"inputSchema":     workspaceToolSchema(),
		"securitySchemes": oauthSecuritySchemes(workspaceReadScope),
		"annotations":     map[string]any{"readOnlyHint": true, "destructiveHint": false},
	}
}

func statPathToolDefinition() map[string]any {
	return map[string]any{
		"name":            statPathToolName,
		"description":     "Return structured metadata for one relative path in an approved workspace without reading file content. The path may be '.' for the workspace root.",
		"inputSchema":     workspaceToolSchema(),
		"securitySchemes": oauthSecuritySchemes(workspaceReadScope),
		"annotations":     map[string]any{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true},
	}
}

func findPathsToolDefinition() map[string]any {
	return map[string]any{
		"name":        findPathsToolName,
		"description": "Find files and directories using Go path.Match semantics against the relative path or basename. The optional root defaults to '.', symlinks are reported but never traversed, and results are bounded.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":  map[string]any{"type": "string"},
				"root":        map[string]any{"type": "string"},
				"pattern":     map[string]any{"type": "string"},
				"max_results": map[string]any{"type": "integer", "minimum": 1, "maximum": workspace.MaxFindResults},
				"max_depth":   map[string]any{"type": "integer", "minimum": 1, "maximum": workspace.MaxFindDepth},
			},
			"required":             []string{"session_id", "pattern"},
			"additionalProperties": false,
		},
		"securitySchemes": oauthSecuritySchemes(workspaceReadScope),
		"annotations":     map[string]any{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true},
	}
}

func searchTextToolDefinition() map[string]any {
	return map[string]any{
		"name":        searchTextToolName,
		"description": "Search literal UTF-8 text in bounded regular files under an approved workspace. The optional root defaults to '.', binary files and symlinks are skipped, and results include relative paths, lines, columns and bounded snippets.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":  map[string]any{"type": "string"},
				"root":        map[string]any{"type": "string"},
				"query":       map[string]any{"type": "string"},
				"max_results": map[string]any{"type": "integer", "minimum": 1, "maximum": workspace.MaxSearchMatches},
			},
			"required":             []string{"session_id", "query"},
			"additionalProperties": false,
		},
		"securitySchemes": oauthSecuritySchemes(workspaceReadScope),
		"annotations":     map[string]any{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true},
	}
}

// authorizeWorkspaceCall autentica antes de interpretar argumentos e reaproveita
// exatamente a mesma fronteira de identidade nas duas ferramentas.
func (a *readToolAccess) authorizeWorkspaceCall(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) (VerifiedIdentity, map[string]json.RawMessage, bool) {
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
		return VerifiedIdentity{}, nil, false
	}
	var arguments map[string]json.RawMessage
	if json.Unmarshal(raw, &arguments) != nil || arguments == nil {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return VerifiedIdentity{}, nil, false
	}
	return identity, arguments, true
}

func (a *readToolAccess) call(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, arguments, ok := a.authorizeWorkspaceCall(w, ctx, id, raw)
	if !ok {
		return
	}
	if len(arguments) != 2 {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	relative, pathOK := requiredString(arguments, "path")
	if !sessionOK || !pathOK || sessionID == "" || relative == "" {
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

func (a *readToolAccess) list(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, arguments, ok := a.authorizeWorkspaceCall(w, ctx, id, raw)
	if !ok {
		return
	}
	if len(arguments) != 2 {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	relative, pathOK := requiredString(arguments, "path")
	if !sessionOK || !pathOK || sessionID == "" || relative == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
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

func (a *readToolAccess) stat(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, arguments, ok := a.authorizeWorkspaceCall(w, ctx, id, raw)
	if !ok {
		return
	}
	if len(arguments) != 2 {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	relative, pathOK := requiredString(arguments, "path")
	if !sessionOK || !pathOK || sessionID == "" || relative == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	result, err := a.statter.StatPath(identity.OwnerSubject, identity.ClientID, sessionID, relative)
	if err != nil {
		replyStructuredWorkspaceError(w, id, "Workspace stat unavailable or not authorized.")
		return
	}
	replyStructured(w, id, result, false)
}

func (a *readToolAccess) find(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, arguments, ok := a.authorizeWorkspaceCall(w, ctx, id, raw)
	if !ok {
		return
	}
	if !hasOnly(arguments, "session_id", "root", "pattern", "max_results", "max_depth") || len(arguments) < 2 || len(arguments) > 5 {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	pattern, patternOK := requiredString(arguments, "pattern")
	root, rootOK := optionalRoot(arguments)
	maxResults, resultsOK := optionalInt(arguments, "max_results", workspace.MaxFindResults)
	maxDepth, depthOK := optionalInt(arguments, "max_depth", workspace.MaxFindDepth)
	if !sessionOK || !patternOK || !rootOK || !resultsOK || !depthOK || sessionID == "" || pattern == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	result, err := a.finder.FindPaths(identity.OwnerSubject, identity.ClientID, sessionID, root, pattern, maxResults, maxDepth)
	if err != nil {
		replyStructuredWorkspaceError(w, id, "Workspace path search unavailable or not authorized.")
		return
	}
	replyStructured(w, id, result, false)
}

func (a *readToolAccess) search(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, arguments, ok := a.authorizeWorkspaceCall(w, ctx, id, raw)
	if !ok {
		return
	}
	if !hasOnly(arguments, "session_id", "root", "query", "max_results") || len(arguments) < 2 || len(arguments) > 4 {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	sessionID, sessionOK := requiredString(arguments, "session_id")
	query, queryOK := requiredString(arguments, "query")
	root, rootOK := optionalRoot(arguments)
	maxResults, resultsOK := optionalInt(arguments, "max_results", workspace.MaxSearchMatches)
	if !sessionOK || !queryOK || !rootOK || !resultsOK || sessionID == "" || query == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	result, err := a.searcher.SearchText(identity.OwnerSubject, identity.ClientID, sessionID, root, query, maxResults)
	if err != nil {
		replyStructuredWorkspaceError(w, id, "Workspace text search unavailable or not authorized.")
		return
	}
	replyStructured(w, id, result, false)
}

func hasOnly(arguments map[string]json.RawMessage, names ...string) bool {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	for name := range arguments {
		if _, ok := allowed[name]; !ok {
			return false
		}
	}
	return true
}

func optionalRoot(arguments map[string]json.RawMessage) (string, bool) {
	if _, ok := arguments["root"]; !ok {
		return ".", true
	}
	root, ok := requiredString(arguments, "root")
	return root, ok && root != ""
}

func optionalInt(arguments map[string]json.RawMessage, name string, defaultValue int) (int, bool) {
	raw, ok := arguments[name]
	if !ok {
		return defaultValue, true
	}
	if string(raw) == "null" {
		return 0, false
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return 0, false
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.Atoi(number.String())
	if err != nil || parsed < 1 || parsed > defaultValue {
		return 0, false
	}
	return parsed, true
}

func replyStructured(w http.ResponseWriter, id any, payload any, isError bool) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		fail(w, http.StatusOK, id, -32603, "Workspace result unavailable")
		return
	}
	reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content":           []any{map[string]any{"type": "text", "text": string(encoded)}},
		"structuredContent": payload,
		"isError":           isError,
	}})
}

func replyStructuredWorkspaceError(w http.ResponseWriter, id any, message string) {
	replyStructuredErrorWithMessage(w, id, message, map[string]any{"status": "unavailable_or_not_authorized"})
}

func replyStructuredErrorWithMessage(w http.ResponseWriter, id any, message string, payload any) {
	reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content":           []any{map[string]any{"type": "text", "text": message}},
		"structuredContent": payload,
		"isError":           true,
	}})
}
