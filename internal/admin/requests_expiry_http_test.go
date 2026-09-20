package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
)

type expiredOAuthRequests struct {
	retained bool
}

func (f *expiredOAuthRequests) ListRequestSnapshots() []auth.RequestSnapshot {
	if !f.retained {
		return nil
	}
	return []auth.RequestSnapshot{{ID: strings.Repeat("e", 22), Version: 2, Status: "EXPIRED"}}
}

func (f *expiredOAuthRequests) GetRequestSnapshot(id string) (auth.RequestSnapshot, error) {
	if !f.retained || id != strings.Repeat("e", 22) {
		return auth.RequestSnapshot{}, auth.ErrOAuthRequestNotFound
	}
	return auth.RequestSnapshot{ID: id, Version: 2, Status: "EXPIRED"}, nil
}

func (f *expiredOAuthRequests) DecideAndSnapshot(id string, _ int, _ string) (auth.RequestSnapshot, error) {
	if !f.retained || id != strings.Repeat("e", 22) {
		return auth.RequestSnapshot{}, auth.ErrOAuthRequestNotFound
	}
	return auth.RequestSnapshot{}, auth.ErrOAuthRequestExpired
}

func TestAdminHTTPExpiredDecisionAndRetentionBoundary(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	requests := &expiredOAuthRequests{retained: true}
	handler := gate.HandlerWithRequests(requests)
	bootstrap := adminRequest(handler, http.MethodGet, "/api/admin/v1/session", "", "")
	csrf := adminResponse(t, bootstrap)["csrf_token"].(string)
	payload, err := json.Marshal(map[string]string{"pairing_code": code, "passphrase": testPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	paired := adminRequest(handler, http.MethodPost, "/api/admin/v1/pair", string(payload), csrf, responseCookie(t, bootstrap, bootstrapCookie))
	if paired.Code != 201 {
		t.Fatalf("pair failed: %d", paired.Code)
	}
	ownerCookie := responseCookie(t, paired, adminCookie)
	ownerCSRF := adminResponse(t, paired)["csrf_token"].(string)
	id := strings.Repeat("e", 22)
	path := fmt.Sprintf("/api/admin/v1/requests/%s", id)
	if got := adminRequest(handler, http.MethodGet, path, "", "", ownerCookie); got.Code != 200 || !strings.Contains(got.Body.String(), `"status":"EXPIRED"`) {
		t.Fatalf("retained expiry not observable: %d %s", got.Code, got.Body.String())
	}
	decision := `{"decision":"approve","expected_version":1}`
	if got := adminRequest(handler, http.MethodPost, path+"/decision", decision, "invalid", ownerCookie); got.Code != 403 {
		t.Fatalf("expiry bypassed CSRF: %d", got.Code)
	}
	if got := adminRequest(handler, http.MethodPost, path+"/decision", decision, ownerCSRF, ownerCookie); got.Code != 410 || !strings.Contains(got.Body.String(), `"REQUEST_EXPIRED"`) {
		t.Fatalf("expired decision did not return 410: %d %s", got.Code, got.Body.String())
	}
	requests.retained = false
	if got := adminRequest(handler, http.MethodGet, path, "", "", ownerCookie); got.Code != 404 {
		t.Fatalf("discarded snapshot still returned: %d", got.Code)
	}
	if got := adminRequest(handler, http.MethodPost, path+"/decision", decision, ownerCSRF, ownerCookie); got.Code != 404 {
		t.Fatalf("discarded request did not return 404: %d", got.Code)
	}
}
