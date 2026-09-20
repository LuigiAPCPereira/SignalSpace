package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// publicSecurityCall exerce o roteador público por um socket HTTP real.
// Host explícito representa a origem HTTPS publicada, sem iniciar um túnel.
func publicSecurityCall(t *testing.T, client *http.Client, base, host, method, path, body, origin string, cookie *http.Cookie) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, string(payload)
}

func TestPublicOAuthStatusHTTPHeadersAndRequestIsolation(t *testing.T) {
	handler, authorization, err := embeddedHandler(readTestResource, filepath.Join(t.TempDir(), "identity"))
	if err != nil {
		t.Fatal(err)
	}
	defer authorization.Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	const host = "signalspace.example"
	registration := fmt.Sprintf(`{"client_name":"Public status security test","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	registered, body := publicSecurityCall(t, client, server.URL, host, "POST", "/register", registration, "", nil)
	var oauthClient struct {
		ID string `json:"client_id"`
	}
	if registered.StatusCode != http.StatusCreated || json.Unmarshal([]byte(body), &oauthClient) != nil || oauthClient.ID == "" {
		t.Fatalf("registration: %d %s", registered.StatusCode, body)
	}
	challenge := sha256.Sum256([]byte(readTestVerifier))
	query := url.Values{
		"client_id": {oauthClient.ID}, "redirect_uri": {readTestCallback}, "response_type": {"code"},
		"scope": {"signalspace:diagnostic"}, "resource": {readTestResource},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"}, "state": {"first-random-state-for-public-status"},
	}
	consentPath := "/authorize?" + query.Encode()
	consent, page := publicSecurityCall(t, client, server.URL, host, "GET", consentPath, "", "", nil)
	if consent.StatusCode != http.StatusOK || !strings.Contains(consent.Header.Get("Content-Security-Policy"), "default-src 'none'") || !strings.Contains(consent.Header.Get("Content-Security-Policy"), "form-action 'self'") || !strings.Contains(consent.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") || strings.Contains(consent.Header.Get("Content-Security-Policy"), "script-src 'unsafe-inline'") || consent.Header.Get("X-Frame-Options") != "DENY" || consent.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("unsafe consent security headers or page: %d %v", consent.StatusCode, consent.Header)
	}
	firstMatch := regexp.MustCompile(`approve ([A-Za-z0-9_-]{22})`).FindStringSubmatch(page)
	if len(firstMatch) != 2 || len(consent.Cookies()) != 1 {
		t.Fatal("first request or OAuth cookie absent")
	}
	firstID, firstCookie := firstMatch[1], consent.Cookies()[0]
	query.Set("state", "second-random-state-for-public-status")
	other, otherPage := publicSecurityCall(t, client, server.URL, host, "GET", "/authorize?"+query.Encode(), "", "", nil)
	otherMatch := regexp.MustCompile(`approve ([A-Za-z0-9_-]{22})`).FindStringSubmatch(otherPage)
	if other.StatusCode != http.StatusOK || len(otherMatch) != 2 || otherMatch[1] == firstID || len(other.Cookies()) != 1 {
		t.Fatal("second independent OAuth request absent")
	}
	statusPath := "/authorize/status?request_id=" + firstID
	status, payload := publicSecurityCall(t, client, server.URL, host, "GET", statusPath, "", "", firstCookie)
	var data map[string]any
	if status.StatusCode != http.StatusOK || json.Unmarshal([]byte(payload), &data) != nil || len(data) != 3 || data["status"] != "PENDING" || data["expires_at"] == "" || data["server_time"] == "" {
		t.Fatalf("public status response: %d %s", status.StatusCode, payload)
	}
	if status.Header.Get("Cache-Control") != "no-store" || status.Header.Get("Referrer-Policy") != "no-referrer" || status.Header.Get("X-Frame-Options") != "DENY" || status.Header.Get("X-Content-Type-Options") != "nosniff" || status.Header.Get("Cross-Origin-Resource-Policy") != "same-origin" || !strings.Contains(status.Header.Get("Content-Security-Policy"), "default-src 'none'") || !strings.Contains(status.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") || status.Header.Get("Access-Control-Allow-Origin") != "" || len(status.Cookies()) != 0 {
		t.Fatalf("unsafe status headers: %v", status.Header)
	}
	if strings.Contains(payload, oauthClient.ID) || strings.Contains(payload, readTestVerifier) || strings.Contains(payload, firstCookie.Value) {
		t.Fatal("public status disclosed OAuth secrets or client metadata")
	}
	for _, tc := range []struct {
		name, method, path, host, origin string
		cookie                           *http.Cookie
		want                             int
	}{
		{"anonymous", "GET", statusPath, host, "", nil, 403},
		{"other_cookie", "GET", statusPath, host, "", other.Cookies()[0], 403},
		{"other_id", "GET", "/authorize/status?request_id=" + otherMatch[1], host, "", firstCookie, 403},
		{"unknown_id", "GET", "/authorize/status?request_id=ABCDEFGHIJKLMNOPQRSTUV", host, "", firstCookie, 404},
		{"duplicate_id", "GET", statusPath + "&request_id=" + firstID, host, "", firstCookie, 400},
		{"extra_parameter", "GET", statusPath + "&client_id=leak", host, "", firstCookie, 400},
		{"foreign_host", "GET", statusPath, "attacker.example", "", firstCookie, 403},
		{"foreign_origin", "GET", statusPath, host, "https://attacker.example", firstCookie, 403},
		{"post_cannot_decide", "POST", statusPath, host, readTestResource, firstCookie, 403},
		{"admin_cookie_only", "GET", statusPath, host, "", &http.Cookie{Name: "signalspace_admin_session", Value: "not-oauth"}, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, _ := publicSecurityCall(t, client, server.URL, tc.host, tc.method, tc.path, "", tc.origin, tc.cookie)
			if response.StatusCode != tc.want || response.Header.Get("Access-Control-Allow-Origin") != "" || len(response.Cookies()) != 0 {
				t.Fatalf("boundary returned %d, want %d", response.StatusCode, tc.want)
			}
		})
	}
	if snapshot, err := authorization.GetRequestSnapshot(firstID); err != nil || snapshot.Status != "PENDING" {
		t.Fatalf("polling changed OAuth decision: %+v %v", snapshot, err)
	}
	if err := authorization.DecideTerminal(firstID, true); err != nil {
		t.Fatal(err)
	}
	approved, approvedBody := publicSecurityCall(t, client, server.URL, host, "GET", statusPath, "", "", firstCookie)
	if approved.StatusCode != 200 || !strings.Contains(approvedBody, `"status":"APPROVED"`) || len(authorization.IssuedClients()) != 0 {
		t.Fatalf("status did not reflect only decision: %d %s", approved.StatusCode, approvedBody)
	}
	publicAdmin, _ := publicSecurityCall(t, client, server.URL, host, "GET", "/api/admin/v1/session", "", "", &http.Cookie{Name: "signalspace_admin_session", Value: "not-an-admin-session"})
	if publicAdmin.StatusCode != http.StatusNotFound || len(publicAdmin.Cookies()) != 0 {
		t.Fatalf("public handler served admin session: %d", publicAdmin.StatusCode)
	}
}
