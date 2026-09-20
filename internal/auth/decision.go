package auth

import (
	"errors"
	"time"
)

var (
	ErrOAuthRequestNotFound = errors.New("oauth request not found")
	ErrOAuthRequestExpired  = errors.New("oauth request expired")
	ErrOAuthAlreadyDecided  = errors.New("oauth request already decided")
	ErrOAuthStaleRequest    = errors.New("oauth request version changed")
	ErrOAuthGrantRequired   = errors.New("workspace grant required")
	ErrOAuthInvalidDecision = errors.New("invalid oauth decision")
)

// decideLocked exige s.mu; nenhuma decisão cria código, token ou concessão.
func (s *Server) decideLocked(id string, expectedVersion int, decision string, now time.Time) error {
	if !requestID.MatchString(id) || (decision != "approve" && decision != "deny") || expectedVersion < 1 {
		return ErrOAuthInvalidDecision
	}
	p, exists := s.pending[id]
	if !exists {
		if record, ok := s.terminal[id]; ok && now.Before(record.retainUntil) {
			if record.snapshot.Status == "EXPIRED" {
				return ErrOAuthRequestExpired
			}
			return ErrOAuthAlreadyDecided
		}
		return ErrOAuthRequestNotFound
	}
	if !now.Before(p.Expires) {
		s.terminalizeLocked(id, p, "EXPIRED", now)
		delete(s.pending, id)
		return ErrOAuthRequestExpired
	}
	if p.Approved || p.Denied {
		return ErrOAuthAlreadyDecided
	}
	version := p.Version
	if version == 0 {
		version = 1 // compatibilidade com fixtures legados de teste.
	}
	if expectedVersion != version {
		return ErrOAuthStaleRequest
	}
	if decision == "approve" && s.readRequested(p.Scope) && !s.readAllowed(p.ClientID) {
		return ErrOAuthGrantRequired
	}
	p.Approved = decision == "approve"
	p.Denied = decision == "deny"
	p.DecidedAt = now
	p.Version = version + 1
	s.pending[id] = p
	return nil
}

// DecideVersioned compartilha a operação de domínio com terminal e painel.
func (s *Server) DecideVersioned(id string, expectedVersion int, decision string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.decideLocked(id, expectedVersion, decision, time.Now())
}

// DecideTerminal preserva o comando local approve/deny sem contornar a checagem
// da concessão de leitura; terminal e API competem pela mesma decisão atômica.
func (s *Server) DecideTerminal(id string, allow bool) error {
	decision := "deny"
	if allow {
		decision = "approve"
	}
	return s.DecideVersioned(id, 1, decision)
}

// DecideAndSnapshot captura o resultado na mesma região crítica da decisão.
// Assim a conclusão pública concorrente não apaga o pedido entre POST e resposta.
func (s *Server) DecideAndSnapshot(id string, expectedVersion int, decision string) (RequestSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if err := s.decideLocked(id, expectedVersion, decision, now); err != nil {
		return RequestSnapshot{}, err
	}
	return s.snapshotLocked(id, s.pending[id], now), nil
}
