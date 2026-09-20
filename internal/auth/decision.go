package auth

import (
	"errors"
	"time"
)

var (
	ErrOAuthRequestNotFound = errors.New("oauth request not found")
	ErrOAuthRequestExpired = errors.New("oauth request expired")
	ErrOAuthAlreadyDecided = errors.New("oauth request already decided")
	ErrOAuthStaleRequest = errors.New("oauth request version changed")
	ErrOAuthGrantRequired = errors.New("workspace grant required")
	ErrOAuthInvalidDecision = errors.New("invalid oauth decision")
)

// DecideVersioned registra uma única decisão em uma solicitação específica.
// A mutação compartilha o mesmo mutex e mapa usados pelo terminal, pelo OAuth
// público e pela conclusão; não emite código, token ou concessão de workspace.
// A futura API administrativa precisa verificar sessão e CSRF antes da chamada.
func (s *Server) DecideVersioned(id string, expectedVersion int, decision string) error {
	if !requestID.MatchString(id) || (decision != "approve" && decision != "deny") || expectedVersion < 1 {
		return ErrOAuthInvalidDecision
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	p, exists := s.pending[id]
	if !exists {
		return ErrOAuthRequestNotFound
	}
	if !now.Before(p.Expires) {
		return ErrOAuthRequestExpired
	}
	if p.Approved || p.Denied {
		return ErrOAuthAlreadyDecided
	}
	// O modelo existente inicia em versão 1 e permite uma única transição de decisão.
	// A retenção de terminais/COMPLETED requer a evolução da estrutura pending.
	if expectedVersion != 1 {
		return ErrOAuthStaleRequest
	}
	if decision == "approve" && s.readRequested(p.Scope) && !s.readAllowed(p.ClientID) {
		return ErrOAuthGrantRequired
	}
	p.Approved = decision == "approve"
	p.Denied = decision == "deny"
	s.pending[id] = p
	return nil
}
