package workspace

import (
	"errors"
	"strings"
	"sync"
)

var ErrNotAuthorized = errors.New("workspace session not authorized")

// Grants mantém, no máximo, uma raiz concedida pelo terminal ao proprietário e
// ao cliente OAuth selecionado. Nenhuma entrada HTTP cria ou amplia a concessão.
type Grants struct {
	mu       sync.Mutex
	owner    string
	clientID string
	current  *Session
	closed   bool
}

func NewGrants(owner string) (*Grants, error) {
	if owner == "" || strings.TrimSpace(owner) != owner {
		return nil, ErrNotAuthorized
	}
	return &Grants{owner: owner}, nil
}

// validClientID aceita apenas IDs aleatórios do registro OAuth integrado.
func validClientID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// Grant recebe o cliente escolhido pelo proprietário no terminal.
// Uma nova concessão revoga a anterior antes de tornar a nova disponível.
func (g *Grants) Grant(root, clientID string) (string, error) {
	if !validClientID(clientID) {
		return "", ErrNotAuthorized
	}
	opened, err := OpenApprovedRoot(root)
	if err != nil {
		return "", err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		_ = opened.Close()
		return "", ErrClosed
	}
	if g.current != nil {
		if err := g.current.Close(); err != nil {
			_ = opened.Close()
			g.current = nil
			g.clientID = ""
			return "", err
		}
	}
	g.current = opened
	g.clientID = clientID
	return opened.ID(), nil
}

// AllowsClient só é usada pelo emissor OAuth para verificar a concessão corrente.
// Não retorna o ID da sessão e não autoriza uma leitura por si só.
func (g *Grants) AllowsClient(clientID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return !g.closed && g.current != nil && validClientID(clientID) && clientID == g.clientID
}

// ReadText exige proprietário, cliente e sessão exatos. A fronteira de transporte
// deverá fornecer clientID somente após validar assinatura e escopo de leitura.
// Revogação e leitura são serializadas pelo mesmo mutex.
func (g *Grants) ReadText(owner, clientID, id, relative string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return "", ErrClosed
	}
	if owner != g.owner || !validClientID(clientID) || clientID != g.clientID || g.current == nil || id != g.current.ID() {
		return "", ErrNotAuthorized
	}
	return g.current.ReadText(relative)
}

// ListDirectory usa a mesma concessão e o mesmo mutex que a leitura de texto.
// É uma operação interna: não cria ou publica nenhuma ferramenta MCP.
func (g *Grants) ListDirectory(owner, clientID, id, relative string) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, ErrClosed
	}
	if owner != g.owner || !validClientID(clientID) || clientID != g.clientID || g.current == nil || id != g.current.ID() {
		return nil, ErrNotAuthorized
	}
	return g.current.ListDirectory(relative)
}

// Revoke é um comando exclusivamente local, sem rota pública equivalente.
func (g *Grants) Revoke(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return ErrClosed
	}
	if g.current == nil || id != g.current.ID() {
		return ErrNotAuthorized
	}
	err := g.current.Close()
	g.current = nil
	g.clientID = ""
	return err
}

// Close revoga a concessão ao encerrar a instância do servidor.
func (g *Grants) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	g.closed = true
	if g.current == nil {
		return nil
	}
	err := g.current.Close()
	g.current = nil
	g.clientID = ""
	return err
}
