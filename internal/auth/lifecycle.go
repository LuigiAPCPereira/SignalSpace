package auth

import (
	"sort"
	"time"
)

const (
	maxTerminalRequests = 64
	terminalRetention   = 10 * time.Minute
)

// terminalRecord não guarda PKCE, state ou CSRF. O hash mantém somente a
// consulta pública de um pedido expirado/negado vinculada ao cookie original.
type terminalRecord struct {
	snapshot    RequestSnapshot
	sessionHash [32]byte
	retainUntil time.Time
}

// terminalizeLocked exige s.mu e só preserva um resultado durante a janela
// contada da transição real, nunca a partir da próxima consulta HTTP.
func (s *Server) terminalizeLocked(id string, p pending, status string, now time.Time) {
	transition := now
	switch status {
	case "EXPIRED":
		transition = p.Expires
	case "DENIED":
		if !p.DecidedAt.IsZero() {
			transition = p.DecidedAt
		}
	}
	if !now.Before(transition.Add(terminalRetention)) {
		return
	}
	item := s.snapshotLocked(id, p, now)
	item.Status = status
	switch status {
	case "EXPIRED":
		item.Version = 2
		if p.Approved {
			item.Version = 3
		}
	case "COMPLETED":
		item.Version = 3
	case "DENIED":
		item.Version = 2
	}
	// A conclusão remove o cookie público; não reter seu vínculo após o código.
	hash := p.SessionHash
	if status == "COMPLETED" {
		hash = [32]byte{}
	}
	s.terminal[id] = terminalRecord{snapshot: item, sessionHash: hash, retainUntil: transition.Add(terminalRetention)}
	s.pruneTerminalLocked(now)
}

// expireRequestsLocked é chamado sob o mesmo mutex usado por decisão e conclusão.
func (s *Server) expireRequestsLocked(now time.Time) {
	for id, p := range s.pending {
		if now.Before(p.Expires) {
			continue
		}
		status := "EXPIRED"
		if p.Denied {
			status = "DENIED"
		}
		s.terminalizeLocked(id, p, status, now)
		delete(s.pending, id)
	}
	s.pruneTerminalLocked(now)
}

func (s *Server) pruneTerminalLocked(now time.Time) {
	for id, record := range s.terminal {
		if !now.Before(record.retainUntil) {
			delete(s.terminal, id)
		}
	}
	if len(s.terminal) <= maxTerminalRequests {
		return
	}
	ids := make([]string, 0, len(s.terminal))
	for id := range s.terminal {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := s.terminal[ids[i]].retainUntil, s.terminal[ids[j]].retainUntil
		if !a.Equal(b) {
			return a.Before(b)
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids[:len(ids)-maxTerminalRequests] {
		delete(s.terminal, id)
	}
}

// terminalSnapshotLocked reconsulta apenas a situação do grant, sem recriar
// autorização nem emitir código ou tocar o prazo de retenção.
func (s *Server) terminalSnapshotLocked(record terminalRecord) RequestSnapshot {
	item := record.snapshot
	if item.WorkspaceRead.Required {
		item.WorkspaceRead.GrantStatus = "REVOKED"
		if s.readAllowed(item.Client.ID) {
			item.WorkspaceRead.GrantStatus = "ACTIVE"
		}
	}
	return item
}
