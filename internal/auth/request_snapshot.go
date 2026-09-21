package auth

import (
	"sort"
	"time"
)

// RequestClient expõe somente metadados registrados; o nome não atesta o software.
type RequestClient struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Verified    bool   `json:"verified"`
}

// RequestWorkspaceRead descreve a concessão corrente, sem revelar pasta ou sessão.
type RequestWorkspaceRead struct {
	Required    bool   `json:"required"`
	GrantStatus string `json:"grant_status"`
}

// RequestSnapshot é uma cópia sem cookie, PKCE, state, CSRF ou caminhos locais.
type RequestSnapshot struct {
	ID            string               `json:"id"`
	Version       int                  `json:"version"`
	Status        string               `json:"status"`
	Client        RequestClient        `json:"client"`
	RedirectURI   string               `json:"redirect_uri"`
	Scope         string               `json:"scope"`
	WorkspaceRead RequestWorkspaceRead `json:"workspace_read"`
	CreatedAt     time.Time            `json:"created_at"`
	ExpiresAt     time.Time            `json:"expires_at"`
	DecidedAt     *time.Time           `json:"decided_at"`
}

// snapshotLocked exige s.mu e calcula o grant apenas pelo serviço de concessões.
func (s *Server) snapshotLocked(id string, p pending, now time.Time) RequestSnapshot {
	version := p.Version
	if version == 0 {
		version = 1 // Compatibilidade com entradas montadas por testes antigos.
	}
	status := "PENDING"
	switch {
	case p.Denied:
		status = "DENIED"
	case !now.Before(p.Expires):
		status = "EXPIRED"
		version++
	case p.Approved:
		status = "APPROVED"
	}
	read := s.readRequested(p.Scope)
	grantStatus := "NOT_APPLICABLE"
	if read {
		grantStatus = "REVOKED"
		if s.readAllowed(p.ClientID) {
			grantStatus = "ACTIVE"
		}
	}
	created := p.CreatedAt
	if created.IsZero() {
		created = p.Expires.Add(-pendingTTL)
	}
	var decidedAt *time.Time
	if !p.DecidedAt.IsZero() {
		utc := p.DecidedAt.UTC()
		decidedAt = &utc
	}
	return RequestSnapshot{
		ID: id, Version: version, Status: status,
		Client:      RequestClient{ID: p.ClientID, DisplayName: s.clients[p.ClientID].Name, Verified: false},
		RedirectURI: p.Redirect, Scope: p.Scope,
		WorkspaceRead: RequestWorkspaceRead{Required: read, GrantStatus: grantStatus},
		CreatedAt:     created.UTC(), ExpiresAt: p.Expires.UTC(), DecidedAt: decidedAt,
	}
}

// ListRequestSnapshots executa limpeza limitada, sem renovar autorização ou
// prazo. Uma expiração é transição real e passa ao armazenamento terminal.
func (s *Server) ListRequestSnapshots() []RequestSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.clean(now)
	items := make([]RequestSnapshot, 0, len(s.pending)+len(s.terminal))
	for id, p := range s.pending {
		items = append(items, s.snapshotLocked(id, p, now))
	}
	for _, record := range s.terminal {
		items = append(items, s.terminalSnapshotLocked(record))
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].ID < items[j].ID
	})
	return items
}

// GetRequestSnapshot exige ID exato e não confunde 404 com expiração.
func (s *Server) GetRequestSnapshot(id string) (RequestSnapshot, error) {
	if !requestID.MatchString(id) {
		return RequestSnapshot{}, ErrOAuthRequestNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.clean(now)
	if p, ok := s.pending[id]; ok {
		return s.snapshotLocked(id, p, now), nil
	}
	if record, ok := s.terminal[id]; ok {
		return s.terminalSnapshotLocked(record), nil
	}
	return RequestSnapshot{}, ErrOAuthRequestNotFound
}
