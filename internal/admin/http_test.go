package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func adminRequest(handler http.Handler, method, path, body, csrf string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = "localhost:7677"
	if method == http.MethodPost || method == http.MethodDelete {
		r.Header.Set("Origin", AdminOrigin)
		r.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
	}
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func adminResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func responseCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == name && cookie.MaxAge > 0 {
			return cookie
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}

func TestAdminHTTPPairSessionLockAndUnlock(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	handler := gate.Handler()
	first := adminRequest(handler, "GET", "/api/admin/v1/session", "", "")
	if first.Code != 200 {
		t.Fatalf("bootstrap: %d", first.Code)
	}
	model := adminResponse(t, first)
	if model["state"] != "UNPAIRED" || model["authenticated"] != false {
		t.Fatalf("unexpected bootstrap state: %v", model)
	}
	bootstrap := responseCookie(t, first, bootstrapCookie)
	if !bootstrap.HttpOnly || bootstrap.SameSite != http.SameSiteStrictMode || bootstrap.Path != cookiePath || bootstrap.Secure {
		t.Fatal("bootstrap cookie policy mismatch")
	}
	payload, _ := json.Marshal(map[string]string{"pairing_code": code, "passphrase": testPassphrase})
	invalid := adminRequest(handler, "POST", "/api/admin/v1/pair", string(payload), "bad-csrf", bootstrap)
	if invalid.Code != 403 {
		t.Fatalf("CSRF bypass: %d", invalid.Code)
	}
	paired := adminRequest(handler, "POST", "/api/admin/v1/pair", string(payload), model["csrf_token"].(string), bootstrap)
	if paired.Code != 201 || adminResponse(t, paired)["state"] != "AUTHENTICATED" {
		t.Fatalf("pair did not authenticate: %d %s", paired.Code, paired.Body.String())
	}
	adminCookieValue := responseCookie(t, paired, adminCookie)
	if !adminCookieValue.HttpOnly || adminCookieValue.Path != cookiePath || adminCookieValue.SameSite != http.SameSiteStrictMode || adminCookieValue.Secure {
		t.Fatal("administrative cookie policy mismatch")
	}
	if strings.Contains(paired.Body.String(), adminCookieValue.Value) || strings.Contains(paired.Body.String(), code) {
		t.Fatal("secret leaked to JSON body")
	}
	auth := adminRequest(handler, "GET", "/api/admin/v1/session", "", "", adminCookieValue)
	if auth.Code != 200 || adminResponse(t, auth)["authenticated"] != true {
		t.Fatal("administrative session not persisted")
	}
	authCSRF := adminResponse(t, auth)["csrf_token"].(string)
	wrongHostRequest := httptest.NewRequest("GET", "/api/admin/v1/session", nil)
	wrongHostRequest.Host = "public.trycloudflare.com"
	wrongHostRequest.AddCookie(adminCookieValue)
	wrongHost := httptest.NewRecorder()
	handler.ServeHTTP(wrongHost, wrongHostRequest)
	if wrongHost.Code != 403 {
		t.Fatal("admin accepted public hostname")
	}
	locked := adminRequest(handler, "POST", "/api/admin/v1/lock", "{}", authCSRF, adminCookieValue)
	if locked.Code != 200 || adminResponse(t, locked)["state"] != "LOCKED" {
		t.Fatalf("lock failed: %d", locked.Code)
	}
	if stale := adminRequest(handler, "GET", "/api/admin/v1/session", "", "", adminCookieValue); stale.Code != 401 {
		t.Fatal("locked cookie accepted")
	}
	unlockedBootstrap := responseCookie(t, locked, bootstrapCookie)
	unlockPayload, _ := json.Marshal(map[string]string{"passphrase": testPassphrase})
	unlocked := adminRequest(handler, "POST", "/api/admin/v1/unlock", string(unlockPayload), adminResponse(t, locked)["csrf_token"].(string), unlockedBootstrap)
	if unlocked.Code != 200 || adminResponse(t, unlocked)["state"] != "AUTHENTICATED" {
		t.Fatalf("unlock failed: %d", unlocked.Code)
	}
}

func TestAdminHTTPRejectsCrossOriginMalformedAndPublicPaths(t *testing.T) {
	gate, _, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	handler := gate.Handler()
	for _, path := range []string{"/authorize", "/mcp", "/api/admin/v1/unknown", "/admin"} {
		if w := adminRequest(handler, "GET", path, "", ""); w.Code != 404 {
			t.Fatalf("admin fallback on %s: %d", path, w.Code)
		}
	}
	first := adminRequest(handler, "GET", "/api/admin/v1/session", "", "")
	bootstrap := responseCookie(t, first, bootstrapCookie)
	csrf := adminResponse(t, first)["csrf_token"].(string)
	for _, body := range []string{`{"pairing_code":"a","passphrase":"b","extra":true}`, `{"pairing_code":"a"} {}`, `not-json`} {
		if w := adminRequest(handler, "POST", "/api/admin/v1/pair", body, csrf, bootstrap); w.Code != 400 {
			t.Fatalf("invalid JSON accepted: %d", w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/api/admin/v1/pair", strings.NewReader("{}"))
	r.Host = "localhost:7677"
	r.Header.Set("Origin", "https://attacker.invalid")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", csrf)
	r.AddCookie(bootstrap)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("cross-origin pairing reached handler")
	}
}
