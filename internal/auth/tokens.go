package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"time"
)

const (
	maxTokenFamilies = 256
	maxRefreshTokens = 1024
)

var (
	familyIDPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{32}$`)
	refreshHashPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	scopePartPattern   = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
)

// tokenFamily é a autorização persistente da conexão, não uma capability local.
type tokenFamily struct {
	ID        string
	ClientID  string
	Resource  string
	Scope     string
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// refreshToken guarda somente metadados associados ao hash do segredo.
// O valor puro nunca é armazenado, serializado ou incluído em respostas/logs.
type refreshToken struct {
	FamilyID  string
	ExpiresAt time.Time
	Rotated   bool
}

type storedTokenFamily struct {
	ClientID  string `json:"client_id"`
	Resource  string `json:"resource"`
	Scope     string `json:"scope"`
	ExpiresAt int64  `json:"expires_at"`
	RevokedAt int64  `json:"revoked_at,omitempty"`
}

type storedRefreshToken struct {
	FamilyID  string `json:"family_id"`
	ExpiresAt int64  `json:"expires_at"`
	Rotated   bool   `json:"rotated"`
}

func configSupportsScope(config Config, scope string) bool {
	parts := strings.Fields(scope)
	if len(parts) == 0 || strings.Join(parts, " ") != scope {
		return false
	}
	if len(parts) == 1 && (parts[0] == config.Scope || parts[0] == config.CompositionScope) {
		return parts[0] != ""
	}
	if parts[0] != config.Scope || config.Scope == "" {
		return false
	}
	allowed := []string{config.Scope}
	for _, item := range []string{config.ReadScope, config.WriteScope, config.GitScope, config.GitIndexScope, config.GitCommitScope, config.TestScope} {
		if item != "" {
			allowed = append(allowed, item)
		}
	}
	if len(parts) > len(allowed) {
		return false
	}
	last := -1
	for _, part := range parts {
		if !scopePartPattern.MatchString(part) {
			return false
		}
		position := -1
		for index, candidate := range allowed {
			if candidate == part {
				position = index
				break
			}
		}
		if position <= last {
			return false
		}
		last = position
	}
	return true
}

func encodeTokenState(families map[string]tokenFamily, refresh map[string]refreshToken) (map[string]storedTokenFamily, map[string]storedRefreshToken, error) {
	if len(families) > maxTokenFamilies || len(refresh) > maxRefreshTokens {
		return nil, nil, errors.New("OAuth token state exceeds configured limits")
	}
	storedFamilies := make(map[string]storedTokenFamily, len(families))
	for id, family := range families {
		if !familyIDPattern.MatchString(id) || family.ID != id || family.ClientID == "" || family.Resource == "" || family.Scope == "" || family.ExpiresAt.IsZero() {
			return nil, nil, errors.New("OAuth token family state is malformed")
		}
		item := storedTokenFamily{ClientID: family.ClientID, Resource: family.Resource, Scope: family.Scope, ExpiresAt: family.ExpiresAt.Unix()}
		if family.RevokedAt != nil {
			item.RevokedAt = family.RevokedAt.Unix()
		}
		storedFamilies[id] = item
	}
	storedRefresh := make(map[string]storedRefreshToken, len(refresh))
	for hash, token := range refresh {
		if !refreshHashPattern.MatchString(hash) || !familyIDPattern.MatchString(token.FamilyID) || token.ExpiresAt.IsZero() {
			return nil, nil, errors.New("OAuth refresh-token state is malformed")
		}
		if _, ok := families[token.FamilyID]; !ok {
			return nil, nil, errors.New("OAuth refresh-token state references an unknown family")
		}
		storedRefresh[hash] = storedRefreshToken{FamilyID: token.FamilyID, ExpiresAt: token.ExpiresAt.Unix(), Rotated: token.Rotated}
	}
	return storedFamilies, storedRefresh, nil
}

func decodeTokenState(state storedIdentity, config Config) (map[string]tokenFamily, map[string]refreshToken, error) {
	families := make(map[string]tokenFamily, len(state.TokenFamilies))
	refresh := make(map[string]refreshToken, len(state.RefreshTokens))
	if !config.EnableRefreshTokens {
		if len(state.TokenFamilies) != 0 || len(state.RefreshTokens) != 0 {
			return nil, nil, errors.New("OAuth refresh-token state requires explicit lifecycle configuration")
		}
		return families, refresh, nil
	}
	if len(state.TokenFamilies) > maxTokenFamilies || len(state.RefreshTokens) > maxRefreshTokens {
		return nil, nil, errors.New("OAuth token state exceeds configured limits")
	}
	for id, item := range state.TokenFamilies {
		if !familyIDPattern.MatchString(id) || item.ClientID == "" || item.Resource != config.ResourceURL || item.Scope == "" || item.ExpiresAt <= 0 || !configSupportsScope(config, item.Scope) {
			return nil, nil, errors.New("OAuth token family state is malformed")
		}
		if _, ok := state.Clients[item.ClientID]; !ok {
			return nil, nil, errors.New("OAuth token family state references an unknown client")
		}
		family := tokenFamily{ID: id, ClientID: item.ClientID, Resource: item.Resource, Scope: item.Scope, ExpiresAt: time.Unix(item.ExpiresAt, 0)}
		if item.RevokedAt != 0 {
			family.RevokedAt = timePtr(time.Unix(item.RevokedAt, 0))
		}
		families[id] = family
	}
	for hash, item := range state.RefreshTokens {
		if !refreshHashPattern.MatchString(hash) || !familyIDPattern.MatchString(item.FamilyID) || item.ExpiresAt <= 0 {
			return nil, nil, errors.New("OAuth refresh-token state is malformed")
		}
		if _, ok := families[item.FamilyID]; !ok {
			return nil, nil, errors.New("OAuth refresh-token state references an unknown family")
		}
		refresh[hash] = refreshToken{FamilyID: item.FamilyID, ExpiresAt: time.Unix(item.ExpiresAt, 0), Rotated: item.Rotated}
	}
	return families, refresh, nil
}

func timePtr(value time.Time) *time.Time { return &value }

func hashRefreshToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (s *Server) accessTokenTTL() time.Duration {
	if s.config.AccessTokenTTL > 0 {
		return s.config.AccessTokenTTL
	}
	if s.config.EnableRefreshTokens {
		return v2AccessTokenTTL
	}
	return tokenTTL
}

func (s *Server) refreshTokenTTL() time.Duration {
	if s.config.RefreshTokenTTL > 0 {
		return s.config.RefreshTokenTTL
	}
	return defaultRefreshTTL
}

func (s *Server) persistTokensLocked() error {
	if !s.config.EnableRefreshTokens {
		return nil
	}
	return s.store.save(s.config, s.key, s.keyID, s.clients, s.families, s.refresh)
}

func (s *Server) revokeFamilyLocked(familyID string, now time.Time) error {
	family, ok := s.families[familyID]
	if !ok {
		return ErrOAuthTokenFamilyNotFound
	}
	previous := family
	if family.RevokedAt == nil {
		family.RevokedAt = timePtr(now)
		s.families[familyID] = family
	}
	s.refreshIssuedClientLocked(family.ClientID, now)
	if err := s.persistTokensLocked(); err != nil {
		s.families[familyID] = previous
		s.refreshIssuedClientLocked(previous.ClientID, now)
		return err
	}
	return nil
}

func (s *Server) refreshIssuedClientLocked(clientID string, now time.Time) {
	for _, family := range s.families {
		if family.ClientID == clientID && family.RevokedAt == nil && now.Before(family.ExpiresAt) {
			s.issued[clientID] = true
			return
		}
	}
	delete(s.issued, clientID)
}

// ErrOAuthTokenFamilyNotFound evita que uma revogação administrativa trate um
// identificador inexistente como sucesso.
var ErrOAuthTokenFamilyNotFound = errors.New("OAuth token family not found")

// RevokeTokenFamily revoga explicitamente a conexão OAuth v2 e persiste a
// decisão antes de retornar sucesso. Não concede nem remove capabilities locais.
func (s *Server) RevokeTokenFamily(familyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.config.EnableRefreshTokens {
		return ErrOAuthTokenFamilyNotFound
	}
	return s.revokeFamilyLocked(familyID, time.Now())
}

func (s *Server) supportsRefresh(clientID string) bool {
	client, ok := s.clients[clientID]
	if !ok || !s.config.EnableRefreshTokens {
		return false
	}
	if len(client.GrantTypes) == 0 {
		return false
	}
	for _, grantType := range client.GrantTypes {
		if grantType == "refresh_token" {
			return true
		}
	}
	return false
}

func (s *Server) validateTokenFamilyScope(scope string) bool {
	return configSupportsScope(s.config, scope)
}
