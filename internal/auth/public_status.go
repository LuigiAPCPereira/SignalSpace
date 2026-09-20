package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"
)

const publicStatusPath = "/authorize/status"

// PublicStatusHandler consulta exclusivamente o pedido associado ao cookie OAuth.
// Este handler não decide solicitações, não emite códigos e não expõe dados administrativos.
// A composição pública só deve registrar esta rota após validar seu fluxo de ponta a ponta.
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
		p, exists := s.pending[query.Get("request_id")]
		if !exists {
			s.mu.Unlock()
			bad(w, http.StatusNotFound, "not_found")
			return
		}
		cookieHash := hashSession(cookie.Value)
		if subtle.ConstantTimeCompare(p.SessionHash[:], cookieHash) != 1 {
			s.mu.Unlock()
			bad(w, http.StatusForbidden, "access_denied")
			return
		}
		status := "PENDING"
		if !now.Before(p.Expires) {
			status = "EXPIRED"
		} else if p.Denied {
			status = "DENIED"
		} else if p.Approved {
			status = "APPROVED"
		}
		expires := p.Expires
		s.mu.Unlock()
		jsonReply(w, http.StatusOK, map[string]string{
			"status":      status,
			"server_time": now.UTC().Format(time.RFC3339Nano),
			"expires_at":  expires.UTC().Format(time.RFC3339Nano),
		})
	})
}
