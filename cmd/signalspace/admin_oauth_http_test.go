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

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
)

// adminHTTP envia requisições por sockets HTTP reais, mantendo Host e Origin
// canônicos mesmo com portas efêmeras reservadas pelo harness.
func adminHTTP(t *testing.T, client *http.Client, base, host, method, path, body, origin, csrf string, cookie *http.Cookie) (int, string, []*http.Cookie) {
	t.Helper()
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
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
	return response.StatusCode, string(payload), response.Cookies()
}

func adminHTTPCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.MaxAge > 0 {
			return cookie
		}
	}
	t.Fatalf("missing %s cookie", name)
	return nil
}

func TestAdminOAuthRealHTTPDecisionAndReconciliation(t *testing.T) {
	publicHandler, authorization, err := embeddedHandler(readTestResource, filepath.Join(t.TempDir(), "identity"))
	if err != nil {
		t.Fatal(err)
	}
	defer authorization.Close()
	gate, pairingCode, err := admin.NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	public := httptest.NewServer(publicHandler)
	defer public.Close()
	private := httptest.NewServer(admin.NewServer(gate.HandlerWithRequests(authorization)).Handler)
	defer private.Close()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	const publicHost = "signalspace.example"
	const adminHost = "localhost:7677"
	const adminPath = "/api/admin/v1"

	if status, _, _ := adminHTTP(t, client, public.URL, publicHost, "GET", adminPath+"/session", "", "", "", nil); status != 404 {
		t.Fatalf("public server leaked admin session: %d", status)
	}
	if status, _, _ := adminHTTP(t, client, private.URL, adminHost, "GET", "/authorize", "", "", "", nil); status != 404 {
		t.Fatalf("admin server exposed OAuth: %d", status)
	}
	if status, _, _ := adminHTTP(t, client, private.URL, publicHost, "GET", adminPath+"/session", "", "", "", nil); status != 403 {
		t.Fatalf("admin accepted public Host: %d", status)
	}
	if status, _, _ := adminHTTP(t, client, private.URL, adminHost, "GET", adminPath+"/requests", "", "", "", nil); status != 401 {
		t.Fatalf("unauthenticated queue was exposed: %d", status)
	}

	registration := fmt.Sprintf(`{"client_name":"HTTP test client","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	status, body, _ := adminHTTP(t, client, public.URL, publicHost, "POST", "/register", registration, "", "", nil)
	if status != 201 {
		t.Fatalf("registration: %d %s", status, body)
	}
	var registered struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal([]byte(body), &registered); err != nil || registered.ClientID == "" {
		t.Fatalf("invalid client registration: %v", err)
	}
	challenge := sha256.Sum256([]byte(readTestVerifier))
	query := url.Values{
		"client_id": {registered.ClientID}, "redirect_uri": {readTestCallback},
		"response_type": {"code"}, "scope": {"signalspace:diagnostic"},
		"resource": {readTestResource}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"}, "state": {"long-random-state-for-real-http"},
	}
	status, consent, oauthCookies := adminHTTP(t, client, public.URL, publicHost, "GET", "/authorize?"+query.Encode(), "", "", "", nil)
	if status != 200 {
		t.Fatalf("OAuth consent: %d %s", status, consent)
	}
	csrfMatch := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(consent)
	queue := authorization.ListRequestSnapshots()
	if len(queue) != 1 || queue[0].Status != "PENDING" || len(csrfMatch) != 2 || len(oauthCookies) != 1 {
		t.Fatalf("real OAuth request not pending: %+v", queue)
	}
	id := queue[0].ID
	if status, _, _ := adminHTTP(t, client, private.URL, adminHost, "GET", adminPath+"/requests/"+id, "", "", "", oauthCookies[0]); status != 401 {
		t.Fatalf("OAuth cookie authenticated admin: %d", status)
	}

	status, sessionJSON, bootstrapCookies := adminHTTP(t, client, private.URL, adminHost, "GET", adminPath+"/session", "", "", "", nil)
	var bootstrap struct {
		CSRF string `json:"csrf_token"`
	}
	if status != 200 || json.Unmarshal([]byte(sessionJSON), &bootstrap) != nil || bootstrap.CSRF == "" {
		t.Fatalf("bootstrap: %d %s", status, sessionJSON)
	}
	bootstrapCookie := adminHTTPCookie(t, bootstrapCookies, "signalspace_admin_bootstrap")
	pairBody := fmt.Sprintf(`{"pairing_code":%q,"passphrase":"long-local-owner-passphrase"}`, pairingCode)
	status, sessionJSON, pairedCookies := adminHTTP(t, client, private.URL, adminHost, "POST", adminPath+"/pair", pairBody, admin.AdminOrigin, bootstrap.CSRF, bootstrapCookie)
	var owner struct {
		CSRF string `json:"csrf_token"`
	}
	if status != 201 || json.Unmarshal([]byte(sessionJSON), &owner) != nil || owner.CSRF == "" {
		t.Fatalf("pair: %d %s", status, sessionJSON)
	}
	ownerCookie := adminHTTPCookie(t, pairedCookies, "signalspace_admin_session")
	status, list, _ := adminHTTP(t, client, private.URL, adminHost, "GET", adminPath+"/requests", "", "", "", ownerCookie)
	if status != 200 || !strings.Contains(list, id) || strings.Contains(list, readTestVerifier) || strings.Contains(list, csrfMatch[1]) {
		t.Fatalf("authenticated list missing or leaking: %d %s", status, list)
	}
	decisionPath := adminPath + "/requests/" + id + "/decision"
	decision := `{"decision":"approve","expected_version":1}`
	for _, tc := range []struct {
		name, origin, csrf, body string
		expected                 int
	}{
		{"cross_origin", "https://attacker.invalid", owner.CSRF, decision, 403},
		{"bad_csrf", admin.AdminOrigin, "invalid", decision, 403},
		{"stale_version", admin.AdminOrigin, owner.CSRF, `{"decision":"approve","expected_version":2}`, 409},
		{"untrusted_metadata", admin.AdminOrigin, owner.CSRF, `{"decision":"approve","expected_version":1,"client_id":"forged"}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _, _ := adminHTTP(t, client, private.URL, adminHost, "POST", decisionPath, tc.body, tc.origin, tc.csrf, ownerCookie)
			if got != tc.expected {
				t.Fatalf("decision boundary: got %d, want %d", got, tc.expected)
			}
		})
	}
	if status, _, _ := adminHTTP(t, client, private.URL, adminHost, "GET", adminPath+"/requests/unknown", "", "", "", ownerCookie); status != 404 {
		t.Fatalf("unknown request: %d", status)
	}

	// Descarta o corpo de uma resposta HTTP 200 real: o cliente deve reconciliar
	// via GET, jamais repetir POST às cegas após uma resposta não aproveitada.
	req, err := http.NewRequest("POST", private.URL+decisionPath, strings.NewReader(decision))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = adminHost
	req.Header.Set("Origin", admin.AdminOrigin)
	req.Header.Set("X-CSRF-Token", owner.CSRF)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(ownerCookie)
	lost, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if lost.StatusCode != 200 {
		t.Fatalf("decision was not committed: %d", lost.StatusCode)
	}
	_ = lost.Body.Close()
	status, detail, _ := adminHTTP(t, client, private.URL, adminHost, "GET", adminPath+"/requests/"+id, "", "", "", ownerCookie)
	if status != 200 || !strings.Contains(detail, `"status":"APPROVED"`) || !strings.Contains(detail, `"version":2`) {
		t.Fatalf("GET could not reconcile decision: %d %s", status, detail)
	}
	if status, _, _ := adminHTTP(t, client, private.URL, adminHost, "POST", decisionPath, decision, admin.AdminOrigin, owner.CSRF, ownerCookie); status != 409 {
		t.Fatalf("duplicate POST committed: %d", status)
	}
	if len(authorization.IssuedClients()) != 0 {
		t.Fatal("admin decision alone issued OAuth token")
	}
	form := url.Values{"request": {id}, "csrf": {csrfMatch[1]}}
	completeReq, err := http.NewRequest("POST", public.URL+"/authorize/complete", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	completeReq.Host = publicHost
	completeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	completeReq.AddCookie(oauthCookies[0])
	completed, err := client.Do(completeReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = completed.Body.Close()
	if completed.StatusCode != 303 {
		t.Fatalf("OAuth completion did not redirect: %d", completed.StatusCode)
	}
	status, detail, _ = adminHTTP(t, client, private.URL, adminHost, "GET", adminPath+"/requests/"+id, "", "", "", ownerCookie)
	if status != 200 || !strings.Contains(detail, `"status":"COMPLETED"`) {
		t.Fatalf("completed request not retained: %d %s", status, detail)
	}
}
