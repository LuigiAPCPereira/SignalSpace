package admin

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"
)

const (
	adminCookie     = "signalspace_admin_session"
	bootstrapCookie = "signalspace_admin_bootstrap"
	cookiePath      = "/api/admin/v1"
	maxJSONBytes    = 2048
)

func setCookie(w http.ResponseWriter, name, value string, lifetime time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: cookiePath,
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
		MaxAge: int(lifetime.Seconds()),
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Path: cookiePath, MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}

func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

func adminJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func adminError(w http.ResponseWriter, status int, code, message string) {
	adminJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
		"server_time": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func adminFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRateLimited):
		w.Header().Set("Retry-After", "900")
		adminError(w, 429, "RATE_LIMITED", "Limite de tentativas atingido.")
	case errors.Is(err, ErrAlreadyPaired):
		adminError(w, 409, "ALREADY_PAIRED", "Pareamento já realizado.")
	case errors.Is(err, ErrNotPaired):
		adminError(w, 409, "INVALID_STATE", "Pareamento necessário.")
	case errors.Is(err, ErrAccessDenied):
		adminError(w, 403, "ACCESS_DENIED", "Acesso negado.")
	default:
		adminError(w, 503, "TEMPORARILY_UNAVAILABLE", "Serviço indisponível.")
	}
}

func adminBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		adminError(w, 415, "UNSUPPORTED_MEDIA_TYPE", "Envie JSON.")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		adminError(w, 400, "INVALID_REQUEST", "Requisição inválida.")
		return false
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		adminError(w, 400, "INVALID_REQUEST", "Requisição inválida.")
		return false
	}
	return true
}

func sessionModel(session Session) map[string]any {
	return map[string]any{
		"state": "AUTHENTICATED", "authenticated": true,
		"csrf_token": session.CSRF,
		"idle_expires_at": session.IdleUntil.UTC().Format(time.RFC3339Nano),
		"absolute_expires_at": session.AbsoluteAt.UTC().Format(time.RFC3339Nano),
		"server_time": time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func bootstrapModel(bootstrap Bootstrap, paired bool) map[string]any {
	state := "UNPAIRED"
	if paired {
		state = "LOCKED"
	}
	return map[string]any{
		"state": state, "authenticated": false,
		"csrf_token": bootstrap.CSRF,
		"bootstrap_expires_at": bootstrap.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"server_time": time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// Handler publica somente a API administrativa e aplica o isolamento de Host/Origin.
// Nenhum handler retorna dados de solicitações OAuth antes de Verify.
func (g *Gate) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/v1/session", g.sessionHTTP)
	mux.HandleFunc("/api/admin/v1/pair", g.pairHTTP)
	mux.HandleFunc("/api/admin/v1/unlock", g.unlockHTTP)
	mux.HandleFunc("/api/admin/v1/lock", g.lockHTTP)
	mux.HandleFunc("/api/admin/v1/session/refresh", g.refreshHTTP)
	return Handler(mux)
}

func (g *Gate) sessionHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	adminID := cookieValue(r, adminCookie)
	bootstrapID := cookieValue(r, bootstrapCookie)
	if adminID != "" && bootstrapID == "" {
		if session, err := g.Verify(adminID, "", false); err == nil {
			adminJSON(w, 200, sessionModel(session))
			return
		}
	}
	if adminID != "" {
		clearCookie(w, adminCookie)
		clearCookie(w, bootstrapCookie)
		bootstrap, paired, err := g.Bootstrap("")
		if err != nil {
			adminFailure(w, err)
			return
		}
		setCookie(w, bootstrapCookie, bootstrap.Cookie, bootstrapTTL)
		adminJSON(w, 401, map[string]any{
			"error": map[string]string{"code": "AUTH_REQUIRED", "message": "Desbloqueio necessário."},
			"session": bootstrapModel(bootstrap, paired),
			"server_time": time.Now().UTC().Format(time.RFC3339Nano),
		})
		return
	}
	bootstrap, paired, err := g.Bootstrap(bootstrapID)
	if err != nil {
		adminFailure(w, err)
		return
	}
	if bootstrap.Cookie != bootstrapID {
		setCookie(w, bootstrapCookie, bootstrap.Cookie, bootstrapTTL)
	}
	adminJSON(w, 200, bootstrapModel(bootstrap, paired))
}

func (g *Gate) pairHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		Code       string `json:"pairing_code"`
		Passphrase string `json:"passphrase"`
	}
	if !adminBody(w, r, &input) {
		return
	}
	session, err := g.Pair(cookieValue(r, bootstrapCookie), r.Header.Get("X-CSRF-Token"), input.Code, input.Passphrase)
	if err != nil {
		adminFailure(w, err)
		return
	}
	clearCookie(w, bootstrapCookie)
	setCookie(w, adminCookie, session.Cookie, absoluteTTL)
	adminJSON(w, 201, sessionModel(session))
}

func (g *Gate) unlockHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		Passphrase string `json:"passphrase"`
	}
	if !adminBody(w, r, &input) {
		return
	}
	session, err := g.Unlock(cookieValue(r, bootstrapCookie), r.Header.Get("X-CSRF-Token"), input.Passphrase)
	if err != nil {
		adminFailure(w, err)
		return
	}
	clearCookie(w, bootstrapCookie)
	setCookie(w, adminCookie, session.Cookie, absoluteTTL)
	adminJSON(w, 200, sessionModel(session))
}

func (g *Gate) refreshHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var input struct{}
	if !adminBody(w, r, &input) {
		return
	}
	session, err := g.Refresh(cookieValue(r, adminCookie), r.Header.Get("X-CSRF-Token"))
	if err != nil {
		adminError(w, 401, "AUTH_REQUIRED", "Desbloqueio necessário.")
		return
	}
	adminJSON(w, 200, sessionModel(session))
}

func (g *Gate) lockHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var input struct{}
	if !adminBody(w, r, &input) {
		return
	}
	if err := g.Lock(cookieValue(r, adminCookie), r.Header.Get("X-CSRF-Token")); err != nil {
		adminError(w, 401, "AUTH_REQUIRED", "Desbloqueio necessário.")
		return
	}
	clearCookie(w, adminCookie)
	bootstrap, paired, err := g.Bootstrap("")
	if err != nil {
		adminFailure(w, err)
		return
	}
	setCookie(w, bootstrapCookie, bootstrap.Cookie, bootstrapTTL)
	adminJSON(w, 200, bootstrapModel(bootstrap, paired))
}
