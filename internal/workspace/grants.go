package workspace

import (
	"errors"
	"strings"
	"sync"
)

var ErrNotAuthorized = errors.New("workspace session not authorized")

// Grants mantém, no máximo, uma raiz concedida pelo terminal para a identidade
// OAuth da instância. Nenhuma entrada HTTP pode criar ou ampliar essa concessão.
type Grants struct {
	mu      sync.Mutex
	owner   string
	current *Session
	closed  bool
}

func NewGrants(owner string) (*Grants, error) {
	if owner == "" || strings.TrimSpace(owner) != owner {
		return nil, ErrNotAuthorized
	}
	return &Grants{owner: owner}, nil
}

// Grant só deve ser chamado depois da confirmação local do caminho exato.
// Uma nova concessão revoga a anterior antes de tornar a nova disponível.
func (g *Grants) Grant(root string) (string, error) {
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
			return "", err
		}
	}
	g.current = opened
	return opened.ID(), nil
}

// ReadText exige identidade autorizada e ID exato. Revogação e leitura são
// serializadas: quando Revoke retorna, nenhuma leitura antiga permanece ativa.
func (g *Grants) ReadText(owner, id, relative string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return "", ErrClosed
	}
	if owner != g.owner || g.current == nil || id != g.current.ID() {
		return "", ErrNotAuthorized
	}
	return g.current.ReadText(relative)
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
	return err
}
