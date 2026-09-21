package main

import (
	"crypto/rand"
	"encoding/hex"
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

// workspaceConsole recebe somente comandos do stdin do processo local. O MCP
// não recebe o objeto de concessões e não pode criar sessões por HTTP.
type workspaceConsole struct {
	mu                  sync.Mutex
	grants              *workspace.Grants
	issuedClients       func() []auth.ClientInfo
	readEnabled         bool
	pending             *pendingWorkspace
	programmingApproval *workspace.CapabilityApproval
}

func newWorkspaceConsole(authorization *auth.Server) (*workspaceConsole, error) {
	grants, err := workspace.NewGrants(authorization.OwnerSubject())
	if err != nil {
		return nil, err
	}
	return &workspaceConsole{grants: grants, issuedClients: authorization.IssuedClients}, nil
}

func (c *workspaceConsole) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = nil
	if c.programmingApproval != nil {
		c.programmingApproval.Close()
	}
	return c.grants.Close()
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
		case workspace.ScopeTest:
			descriptions = append(descriptions, "execução: go test ./... com privilégios do usuário; não é sandbox")
		}
	}
	return strings.Join(descriptions, "; ")
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
	if !hasArgument || argument == "" {
		if c.programmingApproval != nil {
			fmt.Fprintln(output, "use workspace clients | workspace request <client-id> <absolute-path> | workspace request-programming <client-id> <scope1,scope2,...> <absolute-path> | workspace approve <id> | workspace cancel <id> | workspace approve-programming <id> | workspace cancel-programming <id> | workspace revoke <session-id>")
		} else {
			fmt.Fprintln(output, "use workspace clients | workspace request <client-id> <absolute-path> | workspace approve <id> | workspace cancel <id> | workspace revoke <session-id>")
		}
		return true
	}
	switch operation {
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
		fmt.Fprintf(output, "Local workspace grant created: session=%s client=%s. Revoke using workspace revoke %s\n", id, pending.clientID, id)
		if c.readEnabled {
			fmt.Fprintln(output, "A leitura requer um novo consentimento OAuth com signalspace:workspace.read e o ID da sessão. Revogue com workspace revoke <session-id>.")
		} else {
			fmt.Fprintln(output, "A concessão é interna e local; o MCP continua oferecendo somente connection_diagnostic.")
		}
	case "revoke":
		if err := c.grants.Revoke(argument); err != nil {
			fmt.Fprintln(output, "workspace session not active or already revoked")
		} else {
			fmt.Fprintln(output, "workspace grant revoked")
		}
	default:
		fmt.Fprintln(output, "unknown workspace command")
	}
	return true
}
