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
// A retenção de COMPLETED e o instante exato da decisão ainda dependem da
// evolução da estrutura pending; não publicar este modelo como contrato final.
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
	status, version := "PENDING", 1
	switch {
	case !now.Before(p.Expires):
		status = "EXPIRED"
		version = 2
	case p.Denied:
		status = "DENIED"
		version = 2
	case p.Approved:
		status = "APPROVED"
		version = 2
	}
	read := s.readRequested(p.Scope)
	grantStatus := "NOT_APPLICABLE"
	if read {
		grantStatus = "REVOKED"
		if s.readAllowed(p.ClientID) {
			grantStatus = "ACTIVE"
		}
	}
	return RequestSnapshot{
		ID: id, Version: version, Status: status,
		Client: RequestClient{ID: p.ClientID, DisplayName: s.clients[p.ClientID].Name, Verified: false},
		RedirectURI: p.Redirect, Scope: p.Scope,
		WorkspaceRead: RequestWorkspaceRead{Required: read, GrantStatus: grantStatus},
		CreatedAt: p.Expires.Add(-pendingTTL).UTC(), ExpiresAt: p.Expires.UTC(),
	}
}

// ListRequestSnapshots não executa clean: expiração observável não é um GET mutável.
// O armazenamento existente ainda apaga terminais na limpeza OAuth comum.
func (s *Server) ListRequestSnapshots() []RequestSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	items := make([]RequestSnapshot, 0, len(s.pending))
	for id, p := range s.pending {
		items = append(items, s.snapshotLocked(id, p, now))
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].ID < items[j].ID
	})
	return items
}

// GetRequestSnapshot exige um ID exato e não confunde 404 com expiração.
func (s *Server) GetRequestSnapshot(id string) (RequestSnapshot, error) {
	if !requestID.MatchString(id) {
		return RequestSnapshot{}, ErrOAuthRequestNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[id]
	if !ok {
		return RequestSnapshot{}, ErrOAuthRequestNotFound
	}
	return s.snapshotLocked(id, p, time.Now()), nil
}
