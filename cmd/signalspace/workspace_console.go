package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

const workspaceApprovalTTL = 2 * time.Minute

type pendingWorkspace struct {
	id       string
	root     string
	clientID string
	expires  time.Time
}

type pendingManagedWorkspace struct {
	id          string
	clientID    string
	scopes      []string
	sourceRoot  string
	baseRef     string
	workspaceID string
	resume      bool
	expires     time.Time
}

// workspaceConsole recebe somente comandos do stdin do processo local. O MCP
// não recebe o objeto de concessões e não pode criar sessões por HTTP.
type workspaceConsole struct {
	mu                     sync.Mutex
	grants                 *workspace.Grants
	owner                  string
	issuedClients          func() []auth.ClientInfo
	readEnabled            bool
	pending                *pendingWorkspace
	managedPending         *pendingManagedWorkspace
	managed                *workspace.ManagedWorktreeManager
	programmingApproval    *workspace.CapabilityApproval
	programmingGitReviewer *workspaceGitReviewer
}

func newWorkspaceConsole(authorization *auth.Server, stateDir string) (*workspaceConsole, error) {
	grants, err := workspace.NewGrants(authorization.OwnerSubject())
	if err != nil {
		return nil, err
	}
	managed, err := workspace.NewManagedWorktreeManager(stateDir)
	if err != nil {
		_ = grants.Close()
		return nil, err
	}
	return &workspaceConsole{grants: grants, owner: authorization.OwnerSubject(), issuedClients: authorization.IssuedClients, managed: managed}, nil
}

func (c *workspaceConsole) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = nil
	c.managedPending = nil
	if c.programmingApproval != nil {
		c.programmingApproval.Close()
	}
	if c.programmingGitReviewer != nil {
		c.programmingGitReviewer.Close()
	}
	if c.managed != nil {
		_ = c.managed.Close()
	}
	return c.grants.Close()
}

// managedWorkspaceStable revalida a identidade criada pelo gerenciador. O
// resultado não depende de caminho fornecido pela UI nem transforma checkout
// comum em alvo de política persistente.
func (c *workspaceConsole) managedWorkspaceStable(id string) bool {
	if c == nil || c.managed == nil || id == "" {
		return false
	}
	descriptor, err := c.managed.Descriptor(id)
	if err != nil || descriptor.WorkspaceID != id || descriptor.Mode != workspace.WorkspaceModeWorktree {
		return false
	}
	switch descriptor.State {
	case workspace.ManagedWorkspaceAvailable, workspace.ManagedWorkspaceActive, workspace.ManagedWorkspaceDirty:
		return true
	default:
		return false
	}
}

func localWorkspacePath(raw string) bool {
	if len(raw) == 0 || len(raw) > 4096 || !utf8.ValidString(raw) || !filepath.IsAbs(raw) || filepath.Clean(raw) != raw || raw == "/" {
		return false
	}
	if home, err := os.UserHomeDir(); err != nil || raw == filepath.Clean(home) {
		return false
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// isIssuedClient consulta somente a lista fornecida pelo servidor OAuth local.
// Uma string vinda da ferramenta MCP nunca é usada para selecionar a concessão.
func (c *workspaceConsole) isIssuedClient(id string) bool {
	if c.issuedClients == nil {
		return false
	}
	for _, client := range c.issuedClients() {
		if client.ID == id {
			return true
		}
	}
	return false
}

func (c *workspaceConsole) issuedClient(id string) (auth.ClientInfo, bool) {
	if c.issuedClients == nil {
		return auth.ClientInfo{}, false
	}
	for _, client := range c.issuedClients() {
		if client.ID == id {
			return client, true
		}
	}
	return auth.ClientInfo{}, false
}

func capabilityDescriptions(scopes []string) string {
	descriptions := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		switch scope {
		case workspace.ScopeRead:
			descriptions = append(descriptions, "leitura: acesso aos arquivos permitidos")
		case workspace.ScopeWrite:
			descriptions = append(descriptions, "escrita: modificação de arquivos permitidos")
		case workspace.ScopeGit:
			descriptions = append(descriptions, "Git: inspeção de status/diff potencialmente sensíveis, sem commit/push")
		case workspace.ScopeGitIndex:
			descriptions = append(descriptions, "índice Git: staging/unstaging explícito da managed worktree; sem commit, branch ou push")
		case workspace.ScopeGitCommit:
			descriptions = append(descriptions, "commit Git: cria histórico local staged-only na managed worktree; sem hooks, signing, branch ou push")
		case workspace.ScopeTest:
			descriptions = append(descriptions, "execução: go test ./... com privilégios do usuário; não é sandbox")
		}
	}
	return strings.Join(descriptions, "; ")
}

func (c *workspaceConsole) printWorkspaceStatus(output io.Writer) {
	snapshot, err := c.grants.Snapshot()
	if errors.Is(err, workspace.ErrClosed) {
		fmt.Fprintln(output, "Concessão local: indisponível (instância encerrada).")
		return
	}
	if err != nil {
		fmt.Fprintf(output, "Concessão local: estado indisponível (%v).\n", err)
		return
	}
	if !snapshot.Active {
		fmt.Fprintln(output, "Concessão local: ausente.")
		fmt.Fprintln(output, "Pedidos aguardando confirmação não são concessões e não concedem acesso.")
		c.printWorkspaceStatusLimits(output, nil)
		return
	}

	fmt.Fprintln(output, "Concessão local: ativa.")
	fmt.Fprintf(output, "Session ID: %s\nCliente OAuth: id=%s\n", snapshot.SessionID, snapshot.ClientID)
	if client, ok := c.issuedClient(snapshot.ClientID); ok {
		fmt.Fprintf(output, "Nome declarado: %q (identidade do aplicativo não atestada).\n", client.Name)
	} else {
		fmt.Fprintln(output, "Nome declarado: indisponível na lista atual; isso não permite inferir revogação.")
	}
	fmt.Fprintf(output, "Escopos exatos: %s\n", strings.Join(snapshot.Scopes, " "))
	if snapshot.Mode == workspace.WorkspaceModeWorktree {
		fmt.Fprintf(output, "Managed workspace: id=%s base_ref=%q base_sha=%s dirty_source=%t\n", snapshot.ManagedWorkspaceID, snapshot.BaseRef, snapshot.BaseSHA, snapshot.DirtySource)
	}
	fmt.Fprintf(output, "Revogar: workspace revoke current (ou workspace revoke %s).\n", snapshot.SessionID)
	c.printWorkspaceStatusLimits(output, snapshot.Scopes)
}

func (c *workspaceConsole) printWorkspaceStatusLimits(output io.Writer, scopes []string) {
	fmt.Fprintln(output, "Este estado descreve apenas Grants local; não comprova token OAuth válido nem conexão ou chamada MCP.")
	if !c.readEnabled && c.programmingApproval == nil {
		fmt.Fprintln(output, "Modo diagnóstico: uma concessão interna não publica read_file; o MCP permanece limitado a connection_diagnostic.")
	}
	if c.programmingApproval != nil && (containsScope(scopes, workspace.ScopeWrite) || containsScope(scopes, workspace.ScopeGit) || containsScope(scopes, workspace.ScopeGitIndex) || containsScope(scopes, workspace.ScopeGitCommit) || containsScope(scopes, workspace.ScopeTest)) {
		if c.programmingGitReviewer != nil {
			fmt.Fprintln(output, "Escopos públicos opt-in ativos para READ, WRITE, Git review, Git index e Git commit managed-only; shell, branch e Git remoto permanecem fora da composição.")
		} else {
			fmt.Fprintln(output, "Escopos de programação são experimentais e não estão ativados remotamente.")
		}
	}
}

func containsScope(scopes []string, wanted string) bool {
	for _, scope := range scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

// handleWorkspaceCommand processa somente comandos de workspace no stdin local.
// Retorna false para que a autorização OAuth trate a própria entrada.
func (c *workspaceConsole) handleWorkspaceCommand(line string, output io.Writer) bool {
	if line != "workspace" && !strings.HasPrefix(line, "workspace ") {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	command := strings.TrimPrefix(line, "workspace")
	command = strings.TrimPrefix(command, " ")
	operation, argument, hasArgument := strings.Cut(command, " ")
	if operation == "clients" && !hasArgument {
		if c.issuedClients == nil {
			fmt.Fprintln(output, "Nenhum cliente OAuth disponível.")
			return true
		}
		clients := c.issuedClients()
		if len(clients) == 0 {
			fmt.Fprintln(output, "Nenhum cliente OAuth com token emitido nesta instância.")
		}
		for _, client := range clients {
			fmt.Fprintf(output, "Cliente OAuth: id=%s nome=%q (identidade do aplicativo não atestada)\n", client.ID, client.Name)
		}
		return true
	}
	if operation == "status" {
		if hasArgument || argument != "" {
			fmt.Fprintln(output, "use workspace status")
			return true
		}
		c.printWorkspaceStatus(output)
		return true
	}
	if !hasArgument || argument == "" {
		if c.programmingApproval != nil {
			fmt.Fprintln(output, "use workspace clients | workspace status | workspace git-identity show|set <email> <display-name>|clear | workspace worktrees | workspace request <client-id> <absolute-path> | workspace request-programming <client-id> <scope1,scope2,...> <absolute-path> | workspace request-worktree <client-id> <scope1,scope2,...> <source-root> [base-ref] | workspace request-worktree-resume <client-id> <scope1,scope2,...> <workspace-id> | workspace approve <id> | workspace cancel <id> | workspace approve-programming <id> | workspace cancel-programming <id> | workspace approve-worktree <id> | workspace approve-worktree-resume <id> | workspace cancel-worktree <id> | workspace remove-worktree <workspace-id> | workspace revoke current|<session-id>")
		} else {
			fmt.Fprintln(output, "use workspace clients | workspace status | workspace worktrees | workspace request <client-id> <absolute-path> | workspace request-worktree <client-id> <scope1,scope2,...> <source-root> [base-ref] | workspace request-worktree-resume <client-id> <scope1,scope2,...> <workspace-id> | workspace approve <id> | workspace cancel <id> | workspace approve-worktree <id> | workspace approve-worktree-resume <id> | workspace cancel-worktree <id> | workspace remove-worktree <workspace-id> | workspace revoke current|<session-id>")
		}
		return true
	}
	switch operation {
	case "git-identity":
		if c.managed == nil || !hasArgument {
			fmt.Fprintln(output, "use workspace git-identity show | workspace git-identity set <email> <display-name> | workspace git-identity clear")
			return true
		}
		subcommand, value, hasValue := strings.Cut(argument, " ")
		switch subcommand {
		case "show":
			if hasValue {
				fmt.Fprintln(output, "use workspace git-identity show")
				return true
			}
			identity, err := c.managed.GitIdentity()
			if err != nil {
				if errors.Is(err, workspace.ErrGitIdentityMissing) {
					fmt.Fprintln(output, "Git identity: not configured")
				} else {
					fmt.Fprintf(output, "Git identity unavailable: %v\n", err)
				}
				return true
			}
			fmt.Fprintf(output, "Git identity: name=%q email=%q\n", identity.DisplayName, identity.Email)
		case "set":
			if !hasValue || strings.TrimSpace(value) == "" {
				fmt.Fprintln(output, "use workspace git-identity set <email> <display-name>")
				return true
			}
			email, displayName, valid := strings.Cut(value, " ")
			if !valid || strings.TrimSpace(displayName) == "" {
				fmt.Fprintln(output, "use workspace git-identity set <email> <display-name>")
				return true
			}
			if err := c.managed.SetGitIdentity(email, strings.TrimSpace(displayName)); err != nil {
				fmt.Fprintf(output, "Git identity rejected: %v\n", err)
				return true
			}
			fmt.Fprintln(output, "Git identity configured locally")
		case "clear":
			if hasValue {
				fmt.Fprintln(output, "use workspace git-identity clear")
				return true
			}
			if err := c.managed.ClearGitIdentity(); err != nil {
				fmt.Fprintf(output, "Git identity clear rejected: %v\n", err)
				return true
			}
			fmt.Fprintln(output, "Git identity cleared; future commits fail closed")
		default:
			fmt.Fprintln(output, "use workspace git-identity show | workspace git-identity set <email> <display-name> | workspace git-identity clear")
		}
		return true
	case "worktrees":
		if c.managed == nil {
			fmt.Fprintln(output, "managed worktree lifecycle unavailable")
			return true
		}
		if hasArgument {
			fmt.Fprintln(output, "use workspace worktrees")
			return true
		}
		workspaces, err := c.managed.List()
		if err != nil {
			fmt.Fprintf(output, "managed worktree listing unavailable: %v\n", err)
			return true
		}
		if len(workspaces) == 0 {
			fmt.Fprintln(output, "No managed worktrees.")
			return true
		}
		for _, item := range workspaces {
			fmt.Fprintf(output, "workspace_id=%s state=%s active=%t base_ref=%q base_sha=%s dirty_source=%t\n", item.WorkspaceID, item.State, item.Active, item.BaseRef, item.BaseSHA, item.DirtySource)
		}
		return true
	case "request-worktree":
		if c.managed == nil {
			fmt.Fprintln(output, "managed worktree lifecycle unavailable")
			return true
		}
		clientID, rest, valid := strings.Cut(argument, " ")
		scopeText, sourceAndRef, validScopes := strings.Cut(rest, " ")
		if !valid || !validScopes || clientID == "" || scopeText == "" || sourceAndRef == "" || !c.isIssuedClient(clientID) {
			fmt.Fprintln(output, "use workspace request-worktree <client-id> <scope1,scope2,...> <source-root> [base-ref]")
			return true
		}
		sourceRoot, baseRef := splitManagedSourceAndRef(sourceAndRef)
		if sourceRoot == "" || !localWorkspacePath(sourceRoot) {
			fmt.Fprintln(output, "managed worktree source rejected: use a canonical absolute Git repository root")
			return true
		}
		scopes, err := workspace.NormalizeManagedCapabilities(strings.Split(scopeText, ",")...)
		if err != nil {
			fmt.Fprintf(output, "managed worktree request rejected: %v\n", err)
			return true
		}
		if err := c.managed.ValidateCreateRequest(sourceRoot, baseRef); err != nil {
			fmt.Fprintf(output, "managed worktree request rejected: %v\n", err)
			return true
		}
		pending, err := newPendingManagedWorkspace(clientID, scopes, sourceRoot, baseRef, "", false)
		if err != nil {
			fmt.Fprintf(output, "managed worktree approval unavailable: %v\n", err)
			return true
		}
		c.managedPending = pending
		fmt.Fprintf(output, "Managed worktree solicitada (criação local após confirmação): source=%q client=%s scopes=%s", sourceRoot, clientID, strings.Join(scopes, ","))
		if baseRef != "" {
			fmt.Fprintf(output, " base_ref=%q", baseRef)
		}
		fmt.Fprintf(output, "\nConfirme com workspace approve-worktree %s ou cancele com workspace cancel-worktree %s (expira em 2 minutos).\n", pending.id, pending.id)
		return true
	case "request-worktree-resume":
		if c.managed == nil {
			fmt.Fprintln(output, "managed worktree lifecycle unavailable")
			return true
		}
		clientID, rest, valid := strings.Cut(argument, " ")
		scopeText, workspaceID, validID := strings.Cut(rest, " ")
		if !valid || !validID || !c.isIssuedClient(clientID) {
			fmt.Fprintln(output, "use workspace request-worktree-resume <client-id> <scope1,scope2,...> <workspace-id>")
			return true
		}
		scopes, err := workspace.NormalizeManagedCapabilities(strings.Split(scopeText, ",")...)
		if err != nil {
			fmt.Fprintf(output, "managed worktree resume rejected: %v\n", err)
			return true
		}
		descriptor, err := c.managed.Descriptor(workspaceID)
		if err != nil || descriptor.State == workspace.ManagedWorkspaceMissing || descriptor.State == workspace.ManagedWorkspaceInconsistent || descriptor.State == workspace.ManagedWorkspaceStale {
			if err == nil {
				err = workspace.ErrManagedWorkspaceInconsistent
			}
			fmt.Fprintf(output, "managed worktree resume rejected: %v\n", err)
			return true
		}
		pending, err := newPendingManagedWorkspace(clientID, scopes, "", "", workspaceID, true)
		if err != nil {
			fmt.Fprintf(output, "managed worktree approval unavailable: %v\n", err)
			return true
		}
		c.managedPending = pending
		fmt.Fprintf(output, "Managed worktree resume solicitada: workspace_id=%s client=%s scopes=%s\nConfirme com workspace approve-worktree-resume %s ou cancele com workspace cancel-worktree %s (expira em 2 minutos).\n", workspaceID, clientID, strings.Join(scopes, ","), pending.id, pending.id)
		return true
	case "approve-worktree", "approve-worktree-resume", "cancel-worktree":
		if c.managed == nil || c.managedPending == nil || argument != c.managedPending.id || time.Now().After(c.managedPending.expires) {
			c.managedPending = nil
			fmt.Fprintln(output, "managed worktree approval missing, expired or invalid")
			return true
		}
		pending := c.managedPending
		c.managedPending = nil
		if operation == "cancel-worktree" {
			fmt.Fprintln(output, "managed worktree request canceled; no worktree or grant created")
			return true
		}
		if (operation == "approve-worktree-resume") != pending.resume {
			fmt.Fprintln(output, "managed worktree approval command does not match request")
			return true
		}
		if !c.isIssuedClient(pending.clientID) {
			fmt.Fprintln(output, "workspace client no longer eligible")
			return true
		}
		workspaceID := pending.workspaceID
		if !pending.resume {
			descriptor, err := c.managed.Create(pending.sourceRoot, pending.baseRef)
			if err != nil {
				fmt.Fprintf(output, "managed worktree creation rejected: %v\n", err)
				return true
			}
			workspaceID = descriptor.WorkspaceID
		}
		sessionID, descriptor, err := c.managed.Activate(workspaceID, pending.clientID, c.grants, pending.scopes...)
		if err != nil {
			fmt.Fprintf(output, "managed worktree activation rejected: %v\n", err)
			return true
		}
		if containsScope(pending.scopes, workspace.ScopeGit) && c.programmingGitReviewer != nil {
			if err := c.programmingGitReviewer.CaptureBaseline(c.owner, pending.clientID, sessionID); err != nil {
				_ = c.grants.Revoke(sessionID)
				_ = c.managed.Deactivate(workspaceID)
				fmt.Fprintf(output, "managed worktree Git baseline rejected: %v\n", err)
				return true
			}
		}
		fmt.Fprintf(output, "Managed worktree grant created: workspace_id=%s state=%s session=%s client=%s scopes=%s. Revoke using workspace revoke %s\n", descriptor.WorkspaceID, descriptor.State, sessionID, pending.clientID, strings.Join(pending.scopes, ","), sessionID)
		return true
	case "remove-worktree":
		if c.managed == nil || !hasArgument || argument == "" || strings.ContainsAny(argument, " \t\r\n") {
			fmt.Fprintln(output, "use workspace remove-worktree <workspace-id>")
			return true
		}
		if err := c.managed.Remove(argument); err != nil {
			fmt.Fprintf(output, "managed worktree removal rejected: %v\n", err)
			return true
		}
		fmt.Fprintln(output, "managed worktree removed")
		return true
	case "request":
		clientID, root, valid := strings.Cut(argument, " ")
		if !valid || !c.isIssuedClient(clientID) {
			fmt.Fprintln(output, "workspace client rejected: complete OAuth and select an ID shown by workspace clients")
			return true
		}
		if !localWorkspacePath(root) {
			fmt.Fprintln(output, "workspace root rejected: use a canonical absolute directory (not a home-wide or root grant)")
			return true
		}
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			fmt.Fprintln(output, "workspace approval unavailable")
			return true
		}
		id := hex.EncodeToString(nonce[:])
		c.pending = &pendingWorkspace{id: id, root: root, clientID: clientID, expires: time.Now().Add(workspaceApprovalTTL)}
		// %q impede que caracteres de controle de um nome de pasta alterem o terminal.
		fmt.Fprintf(output, "Pasta solicitada (somente leitura): %q\nCliente OAuth selecionado: %s\n", root, clientID)
		fmt.Fprintf(output, "Confirme o caminho exato com workspace approve %s ou cancele com workspace cancel %s (expira em 2 minutos).\n", id, id)
		if !c.readEnabled {
			fmt.Fprintln(output, "Nenhuma ferramenta de arquivo foi habilitada no MCP.")
		}
	case "request-programming":
		if c.programmingApproval == nil {
			fmt.Fprintln(output, "workspace programming approval unavailable")
			return true
		}
		clientID, rest, valid := strings.Cut(argument, " ")
		scopeText, root, validRoot := strings.Cut(rest, " ")
		if !valid || !validRoot || clientID == "" || scopeText == "" || root == "" {
			fmt.Fprintln(output, "use workspace request-programming <client-id> <scope1,scope2,...> <absolute-path>")
			return true
		}
		client, ok := c.issuedClient(clientID)
		if !ok {
			fmt.Fprintln(output, "workspace client rejected: complete OAuth and select an ID shown by workspace clients")
			return true
		}
		requested := strings.Split(scopeText, ",")
		pending, err := c.programmingApproval.Request(root, clientID, requested...)
		if err != nil {
			fmt.Fprintf(output, "workspace programming request rejected: %v\n", err)
			return true
		}
		fmt.Fprintf(output, "Pasta solicitada para programação: %q\nCliente OAuth selecionado: %s (%s)\nCapacidades solicitadas: %s\nEfeitos: %s\n", root, client.Name, client.ID, strings.Join(pending.Scopes, ","), capabilityDescriptions(pending.Scopes))
		fmt.Fprintf(output, "Confirme com workspace approve-programming %s ou cancele com workspace cancel-programming %s (expira em 2 minutos).\n", pending.ID, pending.ID)
	case "approve-programming":
		if c.programmingApproval == nil {
			fmt.Fprintln(output, "workspace programming approval unavailable")
			return true
		}
		pending, sessionID, err := c.programmingApproval.Confirm(argument)
		if err != nil {
			fmt.Fprintf(output, "workspace programming approval rejected: %v\n", err)
			return true
		}
		c.deactivateManagedAfterGrant()
		if containsScope(pending.Scopes, workspace.ScopeGit) && c.programmingGitReviewer != nil {
			if err := c.programmingGitReviewer.CaptureBaseline(c.owner, pending.ClientID, sessionID); err != nil {
				_ = c.grants.Revoke(sessionID)
				fmt.Fprintf(output, "workspace Git baseline rejected: %v\n", err)
				return true
			}
		}
		fmt.Fprintf(output, "Local programming workspace grant created: session=%s client=%s scopes=%s. Revoke using workspace revoke %s\n", sessionID, pending.ClientID, strings.Join(pending.Scopes, ","), sessionID)
	case "cancel-programming":
		if c.programmingApproval == nil {
			fmt.Fprintln(output, "workspace programming approval unavailable")
			return true
		}
		if err := c.programmingApproval.Cancel(argument); err != nil {
			fmt.Fprintf(output, "workspace programming cancellation rejected: %v\n", err)
			return true
		}
		fmt.Fprintln(output, "workspace programming request canceled; no grant created")
	case "approve", "cancel":
		if c.pending == nil || argument != c.pending.id || time.Now().After(c.pending.expires) {
			c.pending = nil
			fmt.Fprintln(output, "workspace approval missing, expired or invalid")
			return true
		}
		pending := c.pending
		c.pending = nil
		if operation == "cancel" {
			fmt.Fprintln(output, "workspace request canceled; no grant created")
			return true
		}
		if !c.isIssuedClient(pending.clientID) {
			fmt.Fprintln(output, "workspace client no longer eligible")
			return true
		}
		id, err := c.grants.Grant(pending.root, pending.clientID)
		if err != nil {
			fmt.Fprintf(output, "workspace grant rejected: %v\n", err)
			return true
		}
		c.deactivateManagedAfterGrant()
		fmt.Fprintf(output, "Local workspace grant created: session=%s client=%s. Revoke using workspace revoke %s\n", id, pending.clientID, id)
		if c.readEnabled {
			fmt.Fprintln(output, "A leitura requer um novo consentimento OAuth com signalspace:workspace.read e o ID da sessão. Revogue com workspace revoke <session-id>.")
		} else {
			fmt.Fprintln(output, "A concessão é interna e local; o MCP continua oferecendo somente connection_diagnostic.")
		}
	case "revoke":
		if strings.ContainsAny(argument, " \t\r\n") {
			fmt.Fprintln(output, "use workspace revoke current | workspace revoke <session-id>")
			return true
		}
		revokeCurrent := argument == "current"
		if revokeCurrent {
			snapshot, err := c.grants.Snapshot()
			if errors.Is(err, workspace.ErrClosed) {
				fmt.Fprintln(output, "workspace grant is unavailable because the instance is closed")
				return true
			}
			if err != nil {
				fmt.Fprintf(output, "workspace grant state unavailable: %v\n", err)
				return true
			}
			if !snapshot.Active {
				fmt.Fprintln(output, "no active workspace grant to revoke")
				return true
			}
			argument = snapshot.SessionID
		}
		if err := c.grants.Revoke(argument); errors.Is(err, workspace.ErrNotAuthorized) && revokeCurrent {
			fmt.Fprintln(output, "workspace grant changed or is no longer active; run workspace status and retry")
		} else if errors.Is(err, workspace.ErrClosed) {
			fmt.Fprintln(output, "workspace grant is unavailable because the instance is closed")
		} else if err != nil {
			fmt.Fprintln(output, "workspace session not active or already revoked")
		} else {
			if c.programmingGitReviewer != nil {
				c.programmingGitReviewer.Forget(argument)
			}
			c.deactivateManagedAfterGrant()
			fmt.Fprintln(output, "workspace grant revoked")
		}
	default:
		fmt.Fprintln(output, "unknown workspace command")
	}
	return true
}

func newPendingManagedWorkspace(clientID string, scopes []string, sourceRoot, baseRef, workspaceID string, resume bool) (*pendingManagedWorkspace, error) {
	var rawID [16]byte
	if _, err := rand.Read(rawID[:]); err != nil {
		return nil, err
	}
	return &pendingManagedWorkspace{id: hex.EncodeToString(rawID[:]), clientID: clientID, scopes: append([]string(nil), scopes...), sourceRoot: sourceRoot, baseRef: baseRef, workspaceID: workspaceID, resume: resume, expires: time.Now().Add(workspaceApprovalTTL)}, nil
}

func splitManagedSourceAndRef(value string) (string, string) {
	if localWorkspacePath(value) {
		if info, err := os.Stat(value); err == nil && info.IsDir() {
			return value, ""
		}
	}
	if !strings.Contains(value, " ") {
		return value, ""
	}
	index := strings.LastIndexByte(value, ' ')
	if index <= 0 || index == len(value)-1 {
		return value, ""
	}
	return value[:index], value[index+1:]
}

func (c *workspaceConsole) deactivateManagedAfterGrant() {
	if c.managed == nil {
		return
	}
	if workspaceID := c.managed.ActiveID(); workspaceID != "" {
		_ = c.managed.Deactivate(workspaceID)
	}
}
