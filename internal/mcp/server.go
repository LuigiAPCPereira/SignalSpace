package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"
)

const (
	protocolVersion         = "2025-06-18"
	maxBodyBytes            = 64 * 1024
	toolName                = "connection_diagnostic"
	readToolName            = "read_file"
	listDirectoryToolName   = "list_directory"
	statPathToolName        = "stat_path"
	findPathsToolName       = "find_paths"
	searchTextToolName      = "search_text"
	createDirectoryToolName = "create_directory"
	createTextFileToolName  = "create_text_file"
	writeTextFileToolName   = "write_text_file"
	copyPathToolName        = "copy_path"
	movePathToolName        = "move_path"
	deleteFileToolName      = "delete_file"
	deleteDirectoryToolName = "delete_directory"
	applyPatchToolName      = "apply_patch"
	commitGitIndexToolName  = "commit_git_index"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

func reply(w http.ResponseWriter, status int, body response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, status int, id any, code int, message string) {
	reply(w, status, response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

func validToken(token string) bool {
	if len(token) < 32 {
		return false
	}
	for _, r := range token {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func authorized(header string, expected [32]byte) bool {
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	provided := sha256.Sum256([]byte(strings.TrimPrefix(header, "Bearer ")))
	return subtle.ConstantTimeCompare(provided[:], expected[:]) == 1
}

func validID(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, true // Notificação legítima: sem identificador.
	}
	var id any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&id); err != nil {
		return nil, false
	}
	switch value := id.(type) {
	case string:
		return value, true
	case json.Number:
		if strings.ContainsAny(value.String(), ".eE") {
			return nil, false
		}
		return value, true
	default:
		return nil, false
	}
}

func validObject(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var obj map[string]json.RawMessage
	return json.Unmarshal(raw, &obj) == nil && obj != nil
}

// NewLocalHandler fornece apenas diagnóstico. Não expor por túnel ou proxy público.
func NewLocalHandler(token string, port int) (http.Handler, error) {
	if !validToken(token) {
		return nil, errors.New("SIGNALSPACE_LOCAL_TOKEN must have at least 32 non-whitespace characters")
	}
	if port < 1 || port > 65535 {
		return nil, errors.New("invalid local port")
	}

	secretHash := sha256.Sum256([]byte(token))
	allowedHosts := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", port): true,
		fmt.Sprintf("localhost:%d", port): true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A validação de Host/Origin impede solicitações de páginas externas ao serviço local.
		if !allowedHosts[r.Host] || (r.Header.Get("Origin") != "" && !allowedHosts[strings.TrimPrefix(r.Header.Get("Origin"), "http://")]) ||
			(r.Header.Get("Origin") != "" && !strings.HasPrefix(r.Header.Get("Origin"), "http://")) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path != "/mcp" || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		if !authorized(r.Header.Get("Authorization"), secretHash) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="signalspace-local"`)
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		serveMCP(w, r, "local_diagnostic", nil, nil, nil, nil, nil, nil, nil)
	}), nil
}

// serveMCP processa o protocolo somente após a fronteira de autenticação.
func serveMCP(w http.ResponseWriter, r *http.Request, mode string, onMCPEvent func(string, string), readAccess *readToolAccess, writeAccess *writeToolAccess, gitAccess *gitToolAccess, gitIndexAccess *gitIndexToolAccess, gitCommitAccess *gitCommitToolAccess, testAccess *testToolAccess) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])); mediaType != "application/json" {
		fail(w, http.StatusUnsupportedMediaType, nil, -32600, "Unsupported media type")
		return
	}
	if r.ContentLength > maxBodyBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}

	var msg request
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := decoder.Decode(&msg); err != nil {
		if isOversized(err) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		fail(w, http.StatusBadRequest, nil, -32700, "Parse error")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if isOversized(err) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		fail(w, http.StatusBadRequest, nil, -32600, "Invalid Request")
		return
	}
	id, ok := validID(msg.ID)
	if !ok || msg.JSONRPC != "2.0" || msg.Method == "" {
		fail(w, http.StatusBadRequest, nil, -32600, "Invalid Request")
		return
	}
	if version := r.Header.Get("MCP-Protocol-Version"); (msg.Method != "initialize" && version != protocolVersion) ||
		(version != "" && version != protocolVersion) {
		fail(w, http.StatusBadRequest, nil, -32600, "Unsupported MCP protocol version")
		return
	}
	if len(msg.ID) == 0 {
		// Notificações nunca geram resposta JSON-RPC.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	handle(w, r, msg, id, mode, onMCPEvent, readAccess, writeAccess, gitAccess, gitIndexAccess, gitCommitAccess, testAccess)
}

func isOversized(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func handle(w http.ResponseWriter, r *http.Request, msg request, id any, mode string, onMCPEvent func(string, string), readAccess *readToolAccess, writeAccess *writeToolAccess, gitAccess *gitToolAccess, gitIndexAccess *gitIndexToolAccess, gitCommitAccess *gitCommitToolAccess, testAccess *testToolAccess) {
	switch msg.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if !validObject(msg.Params) || json.Unmarshal(msg.Params, &params) != nil || params.ProtocolVersion == "" {
			fail(w, http.StatusBadRequest, id, -32602, "Invalid params")
			return
		}
		instructions := "Diagnostic only; no development tools are available."
		programmingDiscovery := mode == "oauth_programming"
		if programmingDiscovery {
			instructions = "Programming tools are discoverable; each category requires a separate OAuth scope and active local grant. Git index and commit require a managed SignalSpace worktree; no shell, remote Git or test.run."
		} else if readAccess != nil && readAccess.discoverable {
			instructions = "File reading requires a separate OAuth read scope, an active local workspace grant and its session ID. No editing or commands."
			if readAccess.lister != nil {
				instructions = "File reading and directory listing require a separate OAuth read scope, an active local workspace grant and its session ID. No editing or commands."
			}
			if readAccess.statter != nil || readAccess.finder != nil || readAccess.searcher != nil {
				instructions += " Structured path metadata, bounded path search and literal text search are available without shell."
			}
		}
		if !programmingDiscovery && writeAccess != nil && writeAccess.discoverable {
			if instructions == "Diagnostic only; no development tools are available." {
				instructions = "Workspace text replacement requires a separate OAuth write scope, an active local workspace write grant and its session ID. No commands or Git mutations."
			} else {
				instructions += " Workspace text replacement requires a separate OAuth write scope and an active local workspace write grant; no commands or Git mutations."
			}
			if writeAccess.directoryCreator != nil || writeAccess.textCreator != nil || writeAccess.textUpdater != nil {
				instructions += " Directory creation, create-only text files and hash-preconditioned full-file updates use the same separate write scope."
			}
			if writeAccess.patchApplier != nil {
				instructions += " Structured apply_patch supports bounded create, hash-preconditioned update/delete, move and directory operations after a complete preflight; it has no shell, Git mutation or arbitrary diff parser."
			}
		}
		if !programmingDiscovery && gitAccess != nil && gitAccess.discoverable {
			instructions += " Git review requires a separate OAuth Git review scope and active local Git review grant; it is read-only and never stages, commits or pushes."
		}
		if !programmingDiscovery && gitIndexAccess != nil && gitIndexAccess.discoverable {
			instructions += " Git index staging and unstaging require a separate Git index scope and an active managed SignalSpace worktree; checkout sessions, shell, commit and remote Git remain unavailable."
		}
		if !programmingDiscovery && testAccess != nil && testAccess.discoverable {
			instructions += " Test execution requires a separate OAuth test scope and active local test grant; it runs only go test ./... and is not a process sandbox."
		}
		if !programmingDiscovery && gitCommitAccess != nil && gitCommitAccess.discoverable {
			instructions += " Git commits require a separate signalspace:git.commit scope, a managed worktree and a configured owner identity; commits are staged-only, detached, local and never push or run hooks/signing."
		}
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "signalspace", "version": "0.1.0"},
			"instructions":    instructions,
		}})
	case "ping":
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{}})
	case "tools/list":
		tool := map[string]any{
			"name":        toolName,
			"description": "Check MCP connectivity without accessing files or running commands.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
			"annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false},
		}
		if mode == "oauth_diagnostic" || mode == "oauth_programming" {
			tool["securitySchemes"] = []any{map[string]any{"type": "oauth2", "scopes": []string{diagnosticScope}}}
		}
		tools := []any{tool}
		if readAccess != nil && readAccess.discoverable {
			tools = append(tools, readToolDefinition())
			if readAccess.lister != nil {
				tools = append(tools, listDirectoryToolDefinition())
			}
			if readAccess.statter != nil {
				tools = append(tools, statPathToolDefinition())
			}
			if readAccess.finder != nil {
				tools = append(tools, findPathsToolDefinition())
			}
			if readAccess.searcher != nil {
				tools = append(tools, searchTextToolDefinition())
			}
		}
		if writeAccess != nil && writeAccess.discoverable {
			tools = append(tools, writeToolDefinition())
			if writeAccess.directoryCreator != nil {
				tools = append(tools, createDirectoryToolDefinition())
			}
			if writeAccess.textCreator != nil {
				tools = append(tools, createTextFileToolDefinition())
			}
			if writeAccess.textUpdater != nil {
				tools = append(tools, writeTextFileToolDefinition())
			}
			if writeAccess.copier != nil {
				tools = append(tools, copyPathToolDefinition())
			}
			if writeAccess.mover != nil {
				tools = append(tools, movePathToolDefinition())
			}
			if writeAccess.fileDeleter != nil {
				tools = append(tools, deleteFileToolDefinition())
			}
			if writeAccess.directoryDeleter != nil {
				tools = append(tools, deleteDirectoryToolDefinition())
			}
			if writeAccess.patchApplier != nil {
				tools = append(tools, applyPatchToolDefinition())
			}
		}
		if gitAccess != nil && gitAccess.discoverable && gitAccess.reviewer != nil {
			tools = append(tools, gitReviewToolDefinition())
		}
		if gitAccess != nil && gitAccess.discoverable {
			if gitAccess.statusReader != nil {
				tools = append(tools, gitStatusToolDefinition())
			}
		}
		if gitIndexAccess != nil && gitIndexAccess.discoverable {
			tools = append(tools, stageGitPathsDefinition(), unstageGitPathsDefinition())
		}
		if gitCommitAccess != nil && gitCommitAccess.discoverable {
			tools = append(tools, commitGitIndexDefinition())
		}
		if testAccess != nil && testAccess.discoverable {
			tools = append(tools, testRunToolDefinition())
		}
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"tools": tools,
		}})
		if onMCPEvent != nil {
			onMCPEvent("tools/list", "")
		}
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if !validObject(msg.Params) || json.Unmarshal(msg.Params, &params) != nil || !validObject(params.Arguments) {
			fail(w, http.StatusOK, id, -32602, "Invalid params")
			return
		}
		if params.Name == readToolName && readAccess != nil {
			readAccess.call(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == listDirectoryToolName && readAccess != nil && readAccess.lister != nil {
			readAccess.list(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == statPathToolName && readAccess != nil && readAccess.statter != nil {
			readAccess.stat(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == findPathsToolName && readAccess != nil && readAccess.finder != nil {
			readAccess.find(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == searchTextToolName && readAccess != nil && readAccess.searcher != nil {
			readAccess.search(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == writeToolName && writeAccess != nil {
			writeAccess.call(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == createDirectoryToolName && writeAccess != nil && writeAccess.directoryCreator != nil {
			writeAccess.createDirectory(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == createTextFileToolName && writeAccess != nil && writeAccess.textCreator != nil {
			writeAccess.createTextFile(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == writeTextFileToolName && writeAccess != nil && writeAccess.textUpdater != nil {
			writeAccess.writeTextFile(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == copyPathToolName && writeAccess != nil && writeAccess.copier != nil {
			writeAccess.copyPath(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == movePathToolName && writeAccess != nil && writeAccess.mover != nil {
			writeAccess.movePath(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == deleteFileToolName && writeAccess != nil && writeAccess.fileDeleter != nil {
			writeAccess.deleteFile(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == deleteDirectoryToolName && writeAccess != nil && writeAccess.directoryDeleter != nil {
			writeAccess.deleteDirectory(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == applyPatchToolName && writeAccess != nil && writeAccess.patchApplier != nil {
			writeAccess.applyPatch(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == gitReviewToolName && gitAccess != nil && gitAccess.reviewer != nil {
			gitAccess.call(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == gitStatusToolName && gitAccess != nil && gitAccess.statusReader != nil {
			gitAccess.status(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == stageGitPathsName && gitIndexAccess != nil {
			gitIndexAccess.call(w, r.Context(), id, params.Arguments, true)
			return
		}
		if params.Name == unstageGitPathsName && gitIndexAccess != nil {
			gitIndexAccess.call(w, r.Context(), id, params.Arguments, false)
			return
		}
		if params.Name == commitGitIndexToolName && gitCommitAccess != nil {
			gitCommitAccess.call(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name == testRunToolName && testAccess != nil {
			testAccess.call(w, r.Context(), id, params.Arguments)
			return
		}
		if params.Name != toolName {
			fail(w, http.StatusOK, id, -32602, "Invalid params")
			return
		}
		var args map[string]json.RawMessage
		_ = json.Unmarshal(params.Arguments, &args)
		if len(args) != 0 {
			fail(w, http.StatusOK, id, -32602, "Invalid params")
			return
		}
		// O servidor não consegue atestar que o solicitante é o ChatGPT. Um ID
		// aleatório permite comparar o resultado exibido no chat com o log local.
		var correlation [16]byte
		if _, err := rand.Read(correlation[:]); err != nil {
			fail(w, http.StatusServiceUnavailable, id, -32603, "Diagnostic unavailable")
			return
		}
		diagnosticID := hex.EncodeToString(correlation[:])
		text, _ := json.Marshal(map[string]any{"connected": true, "mode": mode, "diagnosticID": diagnosticID})
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": string(text)}},
			"isError": false,
		}})
		if onMCPEvent != nil {
			onMCPEvent("tools/call", diagnosticID)
		}
	default:
		fail(w, http.StatusOK, id, -32601, "Method not found")
	}
}
