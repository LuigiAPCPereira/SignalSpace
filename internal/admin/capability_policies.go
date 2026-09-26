package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LuigiAPCPereira/SignalSpace/internal/policy"
)

const capabilityPoliciesPath = "/api/admin/v1/capability-policies"

// CapabilityPolicies expõe somente a administração de políticas persistentes;
// a regra continua sendo avaliada pelo engine com grant ativo.
type CapabilityPolicies interface {
	ListPolicies() ([]policy.StoredPolicy, error)
	RevokePolicy(string) error
}

func capabilityPolicyFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errSessionRequired):
		clearCookie(w, adminCookie)
		adminError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "Desbloqueio necessário.")
	case errors.Is(err, errInvalidCSRF):
		adminError(w, http.StatusForbidden, "ACCESS_DENIED", "Acesso negado.")
	case errors.Is(err, policy.ErrPolicyNotFound):
		adminError(w, http.StatusNotFound, "POLICY_NOT_FOUND", "Política não encontrada.")
	case errors.Is(err, policy.ErrInvalidStore), errors.Is(err, policy.ErrStoreUnavailable):
		adminError(w, http.StatusServiceUnavailable, "POLICY_STORE_UNAVAILABLE", "Políticas indisponíveis; nenhuma autorização persistente foi aplicada.")
	default:
		adminError(w, http.StatusServiceUnavailable, "TEMPORARILY_UNAVAILABLE", "Serviço indisponível.")
	}
}

func registerCapabilityPolicyRoutes(mux *http.ServeMux, gate *Gate, policies CapabilityPolicies) {
	mux.HandleFunc(capabilityPoliciesPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var items []policy.StoredPolicy
		err := gate.withOwner(cookieValue(r, adminCookie), "", false, func() error {
			var listErr error
			items, listErr = policies.ListPolicies()
			return listErr
		})
		if err != nil {
			capabilityPolicyFailure(w, err)
			return
		}
		adminJSON(w, http.StatusOK, map[string]any{"policies": items})
	})
	mux.HandleFunc(capabilityPoliciesPath+"/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, capabilityPoliciesPath+"/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cookie := cookieValue(r, adminCookie)
		if _, err := gate.Verify(cookie, "", false); err != nil {
			capabilityPolicyFailure(w, errSessionRequired)
			return
		}
		csrf := r.Header.Get("X-CSRF-Token")
		if _, err := gate.Verify(cookie, csrf, true); err != nil {
			capabilityPolicyFailure(w, errInvalidCSRF)
			return
		}
		err := gate.withOwner(cookie, csrf, true, func() error { return policies.RevokePolicy(id) })
		if err != nil {
			capabilityPolicyFailure(w, err)
			return
		}
		adminJSON(w, http.StatusOK, map[string]any{"revoked": id})
	})
}
