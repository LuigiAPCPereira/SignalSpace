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
	mu            sync.Mutex
	grants        *workspace.Grants
	issuedClients func() []auth.ClientInfo
	readEnabled   bool
	pending       *pendingWorkspace
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
		fmt.Fprintln(output, "use workspace clients | workspace request <client-id> <absolute-path> | workspace approve <id> | workspace cancel <id> | workspace revoke <session-id>")
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
