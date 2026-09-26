package admin

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/approval"
)

const capabilityApprovalsPath = "/api/admin/v1/capability-approvals"

// CapabilityApprovals é uma porta administrativa somente para o domínio de
// capability approval. Ela não expõe o permit nem conhece OAuth internamente.
type CapabilityApprovals interface {
	ListSnapshots() []approval.Snapshot
	GetSnapshot(string) (approval.Snapshot, error)
	DecideSnapshot(string, int, approval.Decision) (approval.Snapshot, error)
}

func capabilityApprovalFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errSessionRequired):
		clearCookie(w, adminCookie)
		adminError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "Desbloqueio necessário.")
	case errors.Is(err, errInvalidCSRF):
		adminError(w, http.StatusForbidden, "ACCESS_DENIED", "Acesso negado.")
	case errors.Is(err, approval.ErrNotFound), errors.Is(err, approval.ErrPermitNotFound):
		adminError(w, http.StatusNotFound, "APPROVAL_NOT_FOUND", "Solicitação não encontrada.")
	case errors.Is(err, approval.ErrExpired), errors.Is(err, approval.ErrPermitExpired):
		adminError(w, http.StatusGone, "APPROVAL_EXPIRED", "Solicitação expirada.")
	case errors.Is(err, approval.ErrAlreadyDecided):
		adminError(w, http.StatusConflict, "ALREADY_DECIDED", "Solicitação já decidida.")
	case errors.Is(err, approval.ErrStaleVersion):
		adminError(w, http.StatusConflict, "STALE_REQUEST", "A solicitação mudou; consulte o estado atual.")
	case errors.Is(err, approval.ErrInvalidDecision), errors.Is(err, approval.ErrInvalidInput),
		errors.Is(err, approval.ErrInvalidCapability), errors.Is(err, approval.ErrInvalidFingerprint),
		errors.Is(err, approval.ErrInvalidSummary):
		adminError(w, http.StatusBadRequest, "INVALID_REQUEST", "Requisição inválida.")
	case errors.Is(err, approval.ErrCapacityExceeded):
		adminError(w, http.StatusTooManyRequests, "CAPACITY_EXCEEDED", "Limite de solicitações atingido.")
	default:
		adminError(w, http.StatusServiceUnavailable, "TEMPORARILY_UNAVAILABLE", "Serviço indisponível.")
	}
}

func registerCapabilityApprovalRoutes(mux *http.ServeMux, gate *Gate, approvals CapabilityApprovals) {
	mux.HandleFunc(capabilityApprovalsPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var items []approval.Snapshot
		err := gate.withOwner(cookieValue(r, adminCookie), "", false, func() error {
			items = approvals.ListSnapshots()
			return nil
		})
		if err != nil {
			capabilityApprovalFailure(w, err)
			return
		}
		adminJSON(w, http.StatusOK, map[string]any{
			"approvals": items, "server_time": time.Now().UTC().Format(time.RFC3339Nano),
			"next_poll_after_ms": 2000,
		})
	})
	mux.HandleFunc(capabilityApprovalsPath+"/", func(w http.ResponseWriter, r *http.Request) {
		suffix := strings.TrimPrefix(r.URL.Path, capabilityApprovalsPath+"/")
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
			var item approval.Snapshot
			err := gate.withOwner(cookieValue(r, adminCookie), "", false, func() error {
				var getErr error
				item, getErr = approvals.GetSnapshot(id)
				return getErr
			})
			if err != nil {
				capabilityApprovalFailure(w, err)
				return
			}
			adminJSON(w, http.StatusOK, item)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cookie := cookieValue(r, adminCookie)
		if _, err := gate.Verify(cookie, "", false); err != nil {
			capabilityApprovalFailure(w, errSessionRequired)
			return
		}
		csrf := r.Header.Get("X-CSRF-Token")
		if _, err := gate.Verify(cookie, csrf, true); err != nil {
			capabilityApprovalFailure(w, errInvalidCSRF)
			return
		}
		var input struct {
			ExpectedVersion int               `json:"expected_version"`
			Decision        approval.Decision `json:"decision"`
		}
		if !adminBody(w, r, &input) {
			return
		}
		var item approval.Snapshot
		err := gate.withOwner(cookie, csrf, true, func() error {
			var decisionErr error
			item, decisionErr = approvals.DecideSnapshot(id, input.ExpectedVersion, input.Decision)
			return decisionErr
		})
		if err != nil {
			capabilityApprovalFailure(w, err)
			return
		}
		adminJSON(w, http.StatusOK, item)
	})
}
