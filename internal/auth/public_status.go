package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"
)

const publicStatusPath = "/authorize/status"

// PublicStatusHandler consulta exclusivamente o pedido associado ao cookie OAuth.
// Não decide solicitações, não emite códigos nem expõe dados administrativos.
func (s *Server) PublicStatusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path != publicStatusPath || r.URL.RawPath != "" || r.URL.IsAbs() || r.Host != strings.TrimPrefix(s.config.Issuer, "https://") || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.config.Issuer) {
			bad(w, http.StatusForbidden, "access_denied")
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		query := r.URL.Query()
		if len(r.URL.RawQuery) > 128 || len(query) != 1 || len(query["request_id"]) != 1 || !requestID.MatchString(query.Get("request_id")) {
			bad(w, http.StatusBadRequest, "invalid_request")
			return
		}
		cookie, err := r.Cookie("signalspace_auth")
		if err != nil || cookie.Value == "" {
			bad(w, http.StatusForbidden, "access_denied")
			return
		}
		now := time.Now()
		s.mu.Lock()
		// Cota global por rota: cabeçalhos de proxy e IP não são identidade.
		quota := s.quotas[publicStatusPath]
		if quota.Started.IsZero() || now.Sub(quota.Started) >= quotaWindow {
			quota = requestQuota{Started: now}
		}
		if quota.Count >= 120 {
			s.mu.Unlock()
			w.Header().Set("Retry-After", "60")
			bad(w, http.StatusTooManyRequests, "slow_down")
			return
		}
		quota.Count++
		s.quotas[publicStatusPath] = quota
		s.clean(now)
		id := query.Get("request_id")
		status := ""
		var expires time.Time
		var originalHash [32]byte
		if p, ok := s.pending[id]; ok {
			status = "PENDING"
			if p.Denied {
				status = "DENIED"
			} else if p.Approved {
				status = "APPROVED"
			}
			expires = p.Expires
			originalHash = p.SessionHash
		} else if record, ok := s.terminal[id]; ok && record.snapshot.Status != "COMPLETED" {
			status = record.snapshot.Status
			expires = record.snapshot.ExpiresAt
			originalHash = record.sessionHash
		}
		if status == "" {
			s.mu.Unlock()
			bad(w, http.StatusNotFound, "not_found")
			return
		}
		if subtle.ConstantTimeCompare(originalHash[:], hashSession(cookie.Value)) != 1 {
			s.mu.Unlock()
			bad(w, http.StatusForbidden, "access_denied")
			return
		}
		s.mu.Unlock()
		jsonReply(w, http.StatusOK, map[string]string{
			"status": status, "server_time": now.UTC().Format(time.RFC3339Nano),
			"expires_at": expires.UTC().Format(time.RFC3339Nano),
		})
	})
}
