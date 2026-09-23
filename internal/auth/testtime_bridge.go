//go:build signalspace_testtime

package auth

import "time"

// TestExpirePendingRequest encurta somente o prazo de um pedido PENDING.
// Este método só existe quando a compilação opta explicitamente pela tag de teste.
func (s *Server) TestExpirePendingRequest(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.pending[id]
	if !ok {
		if _, retained := s.terminal[id]; retained {
			return ErrOAuthAlreadyDecided
		}
		return ErrOAuthRequestNotFound
	}
	if p.Approved || p.Denied {
		return ErrOAuthAlreadyDecided
	}
	now := time.Now()
	if !now.Before(p.Expires) {
		return ErrOAuthRequestExpired
	}
	p.Expires = now.Add(-time.Second)
	s.pending[id] = p
	return nil
}

// TestExpireTerminalRetention encurta somente a retenção de um tombstone EXPIRED.
// Não cria transições nem altera o comportamento da compilação normal.
func (s *Server) TestExpireTerminalRetention(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.terminal[id]
	if !ok {
		return ErrOAuthRequestNotFound
	}
	if record.snapshot.Status != "EXPIRED" {
		return ErrOAuthAlreadyDecided
	}
	record.retainUntil = time.Now().Add(-time.Second)
	s.terminal[id] = record
	return nil
}
