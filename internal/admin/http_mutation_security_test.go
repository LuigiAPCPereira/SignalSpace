package admin

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAdminMutationDistinguishesCSRFAndMissingSession(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	handler := gate.Handler()
	bootstrapResponse := adminRequest(handler, http.MethodGet, "/api/admin/v1/session", "", "")
	bootstrap := responseCookie(t, bootstrapResponse, bootstrapCookie)
	csrf := adminResponse(t, bootstrapResponse)["csrf_token"].(string)
	payload, _ := json.Marshal(map[string]string{"pairing_code": code, "passphrase": testPassphrase})
	paired := adminRequest(handler, http.MethodPost, "/api/admin/v1/pair", string(payload), csrf, bootstrap)
	if paired.Code != http.StatusCreated {
		t.Fatalf("pair failed: %d", paired.Code)
	}
	owner := responseCookie(t, paired, adminCookie)
	ownerCSRF := adminResponse(t, paired)["csrf_token"].(string)

	for _, path := range []string{"/api/admin/v1/session/refresh", "/api/admin/v1/lock"} {
		withoutCookie := adminRequest(handler, http.MethodPost, path, "{}", ownerCSRF)
		if withoutCookie.Code != http.StatusUnauthorized || adminResponse(t, withoutCookie)["error"].(map[string]any)["code"] != "AUTH_REQUIRED" {
			t.Fatalf("missing session on %s: %d %s", path, withoutCookie.Code, withoutCookie.Body.String())
		}
		wrongCSRF := adminRequest(handler, http.MethodPost, path, "{}", "wrong-csrf", owner)
		if wrongCSRF.Code != http.StatusForbidden || adminResponse(t, wrongCSRF)["error"].(map[string]any)["code"] != "ACCESS_DENIED" {
			t.Fatalf("wrong csrf on %s: %d %s", path, wrongCSRF.Code, wrongCSRF.Body.String())
		}
		if _, err := gate.Verify(owner.Value, ownerCSRF, true); err != nil {
			t.Fatalf("CSRF rejection revoked owner on %s: %v", path, err)
		}
	}
	refreshed := adminRequest(handler, http.MethodPost, "/api/admin/v1/session/refresh", "{}", ownerCSRF, owner)
	if refreshed.Code != http.StatusOK {
		t.Fatalf("valid explicit refresh failed: %d", refreshed.Code)
	}
	locked := adminRequest(handler, http.MethodPost, "/api/admin/v1/lock", "{}", ownerCSRF, owner)
	if locked.Code != http.StatusOK {
		t.Fatalf("valid lock failed: %d", locked.Code)
	}
	if stale := adminRequest(handler, http.MethodPost, "/api/admin/v1/session/refresh", "{}", ownerCSRF, owner); stale.Code != http.StatusUnauthorized {
		t.Fatalf("revoked owner refreshed: %d", stale.Code)
	}
}
