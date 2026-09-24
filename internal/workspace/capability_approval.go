package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const CapabilityApprovalTTL = 2 * time.Minute

var (
	ErrCapabilityApprovalUnavailable = errors.New("capability approval unavailable")
	ErrCapabilityApprovalPending     = errors.New("capability approval already pending")
	ErrCapabilityApprovalMissing     = errors.New("capability approval missing, expired or invalid")
	ErrCapabilityClientRejected      = errors.New("workspace client rejected")
	ErrInvalidCapabilities           = errors.New("invalid workspace capabilities")
)

// CapabilityRequest representa uma solicitação local ainda não aprovada. Os
// escopos são normalizados na ordem estável usada pelo emissor OAuth.
type CapabilityRequest struct {
	ID       string
	Root     string
	ClientID string
	Scopes   []string
	Expires  time.Time
}

type capabilityApproval struct {
	request CapabilityRequest
}

// CapabilityApproval é o componente local compartilhado pelo terminal e pelo
// harness. Criar a composição é uma decisão explícita do chamador local; não
// há rota HTTP, leitura de ambiente ou caminho MCP que a ative.
type CapabilityApproval struct {
	mu       sync.Mutex
	grants   *Grants
	eligible func(string) bool
	now      func() time.Time
	pending  *capabilityApproval
	closed   bool
}

// NewCapabilityApproval cria uma fronteira local de seleção e confirmação.
// eligible deve consultar a fonte confiável de clientes emitidos pelo servidor
// OAuth, nunca um client_id vindo de uma chamada MCP.
func NewCapabilityApproval(grants *Grants, eligible func(string) bool) (*CapabilityApproval, error) {
	if grants == nil || eligible == nil {
		return nil, ErrCapabilityApprovalUnavailable
	}
	return &CapabilityApproval{grants: grants, eligible: eligible, now: time.Now}, nil
}

// NormalizeCapabilities valida e ordena capacidades sem adicionar nenhuma
// implicitamente. A ordem corresponde à metadata OAuth: leitura, escrita, Git,
// teste. A ordem de entrada não é uma autoridade e não pode criar duplicatas.
func NormalizeCapabilities(scopes ...string) ([]string, error) {
	return normalizeCapabilities(false, scopes...)
}

// NormalizeManagedCapabilities só é usada pelo fluxo local owner-side de
// managed worktrees. Não é exposta por MCP/HTTP e não deve ser usada pelo
// pedido genérico de programação em checkout comum.
func NormalizeManagedCapabilities(scopes ...string) ([]string, error) {
	return normalizeCapabilities(true, scopes...)
}

func normalizeCapabilities(managed bool, scopes ...string) ([]string, error) {
	if len(scopes) == 0 {
		return nil, ErrInvalidCapabilities
	}
	wanted := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if scope == "" || strings.TrimSpace(scope) != scope {
			return nil, ErrInvalidCapabilities
		}
		switch scope {
		case ScopeRead, ScopeWrite, ScopeGit, ScopeTest:
		case ScopeGitIndex:
			if !managed {
				return nil, ErrInvalidCapabilities
			}
		default:
			return nil, ErrInvalidCapabilities
		}
		if _, exists := wanted[scope]; exists {
			return nil, ErrInvalidCapabilities
		}
		wanted[scope] = struct{}{}
	}
	canonical := []string{ScopeRead, ScopeWrite, ScopeGit, ScopeGitIndex, ScopeTest}
	result := make([]string, 0, len(wanted))
	for _, scope := range canonical {
		if _, exists := wanted[scope]; exists {
			result = append(result, scope)
		}
	}
	return result, nil
}

// ValidateApprovedRoot valida antecipadamente a raiz que será revalidada por
// GrantWithScopes no momento da confirmação. A abertura temporária também
// detecta symlink em qualquer componente sem reter uma autorização.
func ValidateApprovedRoot(root string) error {
	if len(root) == 0 || len(root) > 4096 || !utf8.ValidString(root) || !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" {
		return ErrInvalidRoot
	}
	if home, err := os.UserHomeDir(); err != nil || root == filepath.Clean(home) {
		return ErrInvalidRoot
	}
	for _, r := range root {
		if unicode.IsControl(r) {
			return ErrInvalidRoot
		}
	}
	session, err := OpenApprovedRoot(root)
	if err != nil {
		return err
	}
	return session.Close()
}

// Request registra a seleção sem criar concessão. Existe no máximo uma
// solicitação pendente por instância, e o client_id é revalidado na aprovação.
func (a *CapabilityApproval) Request(root, clientID string, scopes ...string) (CapabilityRequest, error) {
	canonical, err := NormalizeCapabilities(scopes...)
	if err != nil {
		return CapabilityRequest{}, err
	}
	if !validClientID(clientID) || !a.eligible(clientID) {
		return CapabilityRequest{}, ErrCapabilityClientRejected
	}
	if err := ValidateApprovedRoot(root); err != nil {
		return CapabilityRequest{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return CapabilityRequest{}, ErrCapabilityApprovalUnavailable
	}
	if a.pending != nil {
		if !a.now().Before(a.pending.request.Expires) {
			a.pending = nil
		} else {
			return CapabilityRequest{}, ErrCapabilityApprovalPending
		}
	}
	var rawID [16]byte
	if _, err := rand.Read(rawID[:]); err != nil {
		return CapabilityRequest{}, ErrCapabilityApprovalUnavailable
	}
	request := CapabilityRequest{
		ID:       hex.EncodeToString(rawID[:]),
		Root:     root,
		ClientID: clientID,
		Scopes:   append([]string(nil), canonical...),
		Expires:  a.now().Add(CapabilityApprovalTTL),
	}
	a.pending = &capabilityApproval{request: request}
	return cloneCapabilityRequest(request), nil
}

// Confirm consome o identificador de uso único e só então cria a concessão.
// GrantWithScopes revalida a raiz, o cliente e o estado do gerenciador.
func (a *CapabilityApproval) Confirm(id string) (CapabilityRequest, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.pending == nil || a.pending.request.ID != id {
		return CapabilityRequest{}, "", ErrCapabilityApprovalMissing
	}
	if !a.now().Before(a.pending.request.Expires) {
		a.pending = nil
		return CapabilityRequest{}, "", ErrCapabilityApprovalMissing
	}
	request := a.pending.request
	if !validClientID(request.ClientID) || !a.eligible(request.ClientID) {
		a.pending = nil
		return CapabilityRequest{}, "", ErrCapabilityClientRejected
	}
	a.pending = nil
	sessionID, err := a.grants.GrantWithScopes(request.Root, request.ClientID, request.Scopes...)
	if err != nil {
		return CapabilityRequest{}, "", err
	}
	return cloneCapabilityRequest(request), sessionID, nil
}

// Cancel consome somente a solicitação identificada; um identificador errado
// não cancela uma solicitação legítima.
func (a *CapabilityApproval) Cancel(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.pending == nil || a.pending.request.ID != id {
		return ErrCapabilityApprovalMissing
	}
	a.pending = nil
	return nil
}

// Close descarta solicitações pendentes. A revogação da concessão continua
// pertencendo a Grants.Close/Revoke e ocorre separadamente.
func (a *CapabilityApproval) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	a.pending = nil
}

func cloneCapabilityRequest(request CapabilityRequest) CapabilityRequest {
	request.Scopes = append([]string(nil), request.Scopes...)
	return request
}
