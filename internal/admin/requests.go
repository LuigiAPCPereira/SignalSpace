package admin

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
)

const requestsPath = "/api/admin/v1/requests"

var (
	errSessionRequired = errors.New("admin session required")
	errInvalidCSRF     = errors.New("invalid admin csrf")
)

// OAuthRequests é a porta de leitura e decisão; não oferece emissão de token.
type OAuthRequests interface {
	ListRequestSnapshots() []auth.RequestSnapshot
	GetRequestSnapshot(string) (auth.RequestSnapshot, error)
	DecideAndSnapshot(string, int, string) (auth.RequestSnapshot, error)
}

// withOwner mantém a sessão válida durante toda a leitura/decisão.
// Um lock concorrente nunca pode revogar a sessão entre Verify e o commit OAuth.
func (g *Gate) withOwner(cookie, csrf string, decision bool, operation func() error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.cleanupLocked(now)
	key := sha256.Sum256([]byte(cookie))
	session, ok := g.sessions[key]
	if cookie == "" || !ok {
		return errSessionRequired
	}
	if decision && !sameToken(csrf, session.csrf) {
		return errInvalidCSRF
	}
	if err := operation(); err != nil {
		return err
	}
	if decision {
		// Uma decisão bem-sucedida representa atividade deliberada do proprietário.
		session.idleUntil = now.Add(idleTTL)
		if session.idleUntil.After(session.absoluteAt) {
			session.idleUntil = session.absoluteAt
		}
		g.sessions[key] = session
	}
	return nil
}

func requestFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errSessionRequired):
		clearCookie(w, adminCookie)
		adminError(w, 401, "AUTH_REQUIRED", "Desbloqueio necessário.")
	case errors.Is(err, errInvalidCSRF):
		adminError(w, 403, "ACCESS_DENIED", "Acesso negado.")
	case errors.Is(err, auth.ErrOAuthRequestNotFound):
		adminError(w, 404, "REQUEST_NOT_FOUND", "Solicitação não encontrada.")
	case errors.Is(err, auth.ErrOAuthRequestExpired):
		adminError(w, 410, "REQUEST_EXPIRED", "Solicitação expirada.")
	case errors.Is(err, auth.ErrOAuthAlreadyDecided):
		adminError(w, 409, "ALREADY_DECIDED", "Solicitação já decidida.")
	case errors.Is(err, auth.ErrOAuthStaleRequest):
		adminError(w, 409, "STALE_REQUEST", "A solicitação mudou; consulte o estado atual.")
	case errors.Is(err, auth.ErrOAuthGrantRequired):
		adminError(w, 409, "WORKSPACE_GRANT_REQUIRED", "Concessão de workspace necessária.")
	case errors.Is(err, auth.ErrOAuthInvalidDecision):
		adminError(w, 400, "INVALID_REQUEST", "Requisição inválida.")
	default:
		adminError(w, 503, "TEMPORARILY_UNAVAILABLE", "Serviço indisponível.")
	}
}

// HandlerWithRequests mantém as rotas existentes e isola a API de pedidos.
// O modo Quick ainda precisa conectar este handler ao listener administrativo.
func (g *Gate) HandlerWithRequests(requests OAuthRequests) http.Handler {
	return g.HandlerWithRequestsAndCapabilityApprovals(requests, nil)
}

// HandlerWithRequestsAndCapabilityApprovals mantém a fila OAuth e a fila de
// approvals de capability em domínios e rotas independentes.
func (g *Gate) HandlerWithRequestsAndCapabilityApprovals(requests OAuthRequests, approvals CapabilityApprovals) http.Handler {
	if requests == nil {
		if approvals == nil {
			return g.Handler()
		}
	}
	mux := http.NewServeMux()
	if requests != nil {
		mux.HandleFunc(requestsPath, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var items []auth.RequestSnapshot
			err := g.withOwner(cookieValue(r, adminCookie), "", false, func() error {
				items = requests.ListRequestSnapshots()
				return nil
			})
			if err != nil {
				requestFailure(w, err)
				return
			}
			adminJSON(w, 200, map[string]any{
				"requests":           items,
				"server_time":        time.Now().UTC().Format(time.RFC3339Nano),
				"next_poll_after_ms": 2000,
			})
		})
		mux.HandleFunc(requestsPath+"/", func(w http.ResponseWriter, r *http.Request) {
			suffix := strings.TrimPrefix(r.URL.Path, requestsPath+"/")
			parts := strings.Split(suffix, "/")
			if len(parts) == 0 || parts[0] == "" || len(parts) > 2 || (len(parts) == 2 && parts[1] != "decision") {
				http.NotFound(w, r)
				return
			}
			id := parts[0]
			if len(parts) == 1 {
				if r.Method != http.MethodGet {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				var item auth.RequestSnapshot
				err := g.withOwner(cookieValue(r, adminCookie), "", false, func() error {
					var requestErr error
					item, requestErr = requests.GetRequestSnapshot(id)
					return requestErr
				})
				if err != nil {
					requestFailure(w, err)
					return
				}
				adminJSON(w, 200, item)
				return
			}
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			cookie := cookieValue(r, adminCookie)
			if _, err := g.Verify(cookie, "", false); err != nil {
				requestFailure(w, errSessionRequired)
				return
			}
			csrf := r.Header.Get("X-CSRF-Token")
			if _, err := g.Verify(cookie, csrf, true); err != nil {
				requestFailure(w, errInvalidCSRF)
				return
			}
			var input struct {
				Decision        string `json:"decision"`
				ExpectedVersion int    `json:"expected_version"`
			}
			if !adminBody(w, r, &input) {
				return
			}
			var item auth.RequestSnapshot
			err := g.withOwner(cookie, csrf, true, func() error {
				var requestErr error
				item, requestErr = requests.DecideAndSnapshot(id, input.ExpectedVersion, input.Decision)
				return requestErr
			})
			if err != nil {
				requestFailure(w, err)
				return
			}
			adminJSON(w, 200, item)
		})
	}
	if approvals != nil {
		registerCapabilityApprovalRoutes(mux, g, approvals)
	}
	// O handler legado cobre apenas sessões, pareamento e bloqueio.
	mux.Handle("/", g.Handler())
	return Handler(mux)
}
