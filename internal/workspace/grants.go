package workspace

import (
	"errors"
	"strings"
	"sync"
)

var ErrNotAuthorized = errors.New("workspace session not authorized")

const (
	ScopeRead  = "signalspace:workspace.read"
	ScopeWrite = "signalspace:workspace.write"
	ScopeGit   = "signalspace:git.review"
	ScopeTest  = "signalspace:test.run"
)

// ProcessDirectory é a menor porta necessária para operações locais que
// precisam executar uma observação vinculada ao diretório aprovado. Ela não
// expõe a raiz, o descritor ou o ID da sessão.
type ProcessDirectory interface {
	WithProcessDir(func(string) error) error
}

// Grants mantém, no máximo, uma raiz concedida pelo terminal ao proprietário e
// ao cliente OAuth selecionado. Nenhuma entrada HTTP cria ou amplia a concessão.
type Grants struct {
	mu       sync.Mutex
	owner    string
	clientID string
	scopes   map[string]struct{}
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

// Grant recebe o cliente escolhido pelo proprietário no terminal com leitura
// somente. A escrita exige uma concessão explícita de ScopeWrite.
func (g *Grants) Grant(root, clientID string) (string, error) {
	return g.GrantWithScopes(root, clientID, ScopeRead)
}

// GrantWithScopes compõe uma concessão local com capacidades explícitas. Esta
// fronteira não é uma rota remota: o chamador precisa estar no processo local
// que já possui a decisão do proprietário. Escopos desconhecidos falham
// fechado para impedir que uma string futura seja aceita por acidente.
func (g *Grants) GrantWithScopes(root, clientID string, scopes ...string) (string, error) {
	if !validClientID(clientID) {
		return "", ErrNotAuthorized
	}
	allowedScopes := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if scope != ScopeRead && scope != ScopeWrite && scope != ScopeGit && scope != ScopeTest {
			return "", ErrNotAuthorized
		}
		allowedScopes[scope] = struct{}{}
	}
	if len(allowedScopes) == 0 {
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
			g.scopes = nil
			return "", err
		}
	}
	g.current = opened
	g.clientID = clientID
	g.scopes = allowedScopes
	return opened.ID(), nil
}

// WithAuthorizedGitProcessDir autoriza uma observação Git somente com a
// concessão Git corrente e mantém a chamada serializada com edição e revogação.
// A porta é deliberadamente específica: não há equivalente HTTP nem acesso
// MCP direto à Session.
func (g *Grants) WithAuthorizedGitProcessDir(owner, clientID, id string, operation func(ProcessDirectory) error) error {
	if operation == nil {
		return ErrNotAuthorized
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return ErrClosed
	}
	if !g.authorizedLocked(owner, clientID, id, ScopeGit) {
		return ErrNotAuthorized
	}
	return operation(g.current)
}

// WithAuthorizedTestProcessDir autoriza a execução fixa de testes somente com
// a concessão de teste corrente. A chamada permanece serializada com edição,
// revisão Git e revogação; a porta não expõe Session, raiz ou descritor.
func (g *Grants) WithAuthorizedTestProcessDir(owner, clientID, id string, operation func(ProcessDirectory) error) error {
	if operation == nil {
		return ErrNotAuthorized
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return ErrClosed
	}
	if !g.authorizedLocked(owner, clientID, id, ScopeTest) {
		return ErrNotAuthorized
	}
	return operation(g.current)
}

// AllowsClient só é usada pelo emissor OAuth para verificar a concessão de
// leitura corrente. Não retorna o ID da sessão e não autoriza uma leitura por si só.
func (g *Grants) AllowsClient(clientID string) bool {
	return g.AllowsClientScope(clientID, ScopeRead)
}

// AllowsClientScope verifica a capacidade exata da concessão atual sem
// expor a raiz ou o ID da sessão. O emissor OAuth usa esta porta para
// revalidar escopos experimentais antes de concluir e trocar o código.
func (g *Grants) AllowsClientScope(clientID, scope string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return !g.closed && g.current != nil && validClientID(clientID) && clientID == g.clientID && g.hasScopeLocked(scope)
}

func (g *Grants) hasScopeLocked(scope string) bool {
	_, ok := g.scopes[scope]
	return ok
}

func (g *Grants) authorizedLocked(owner, clientID, id, scope string) bool {
	return owner == g.owner && validClientID(clientID) && clientID == g.clientID &&
		g.current != nil && id == g.current.ID() && g.hasScopeLocked(scope)
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
	if !g.authorizedLocked(owner, clientID, id, ScopeRead) {
		return "", ErrNotAuthorized
	}
	return g.current.ReadText(relative)
}

// ReplaceText exige identidade e uma concessão com ScopeWrite, que é distinto
// da leitura. O conteúdo esperado funciona como uma versão otimista local; a
// operação não é exposta pelo MCP nesta etapa.
func (g *Grants) ReplaceText(owner, clientID, id, relative, expected, replacement string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return ErrClosed
	}
	if !g.authorizedLocked(owner, clientID, id, ScopeWrite) {
		return ErrNotAuthorized
	}
	return g.current.ReplaceText(relative, expected, replacement)
}

// ListDirectory usa a mesma concessão e o mesmo mutex que a leitura de texto.
// É uma operação interna: não cria ou publica nenhuma ferramenta MCP.
func (g *Grants) ListDirectory(owner, clientID, id, relative string) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, ErrClosed
	}
	if !g.authorizedLocked(owner, clientID, id, ScopeRead) {
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
	g.scopes = nil
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
	g.scopes = nil
	return err
}
