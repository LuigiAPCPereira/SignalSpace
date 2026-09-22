package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
)

// Uma concessão criada pelo terminal precisa ser revalidada pelo mesmo emissor
// OAuth que serve a API administrativa. Revogar nunca cria um novo código.
func TestAdminOAuthHTTPReadGrantRevocation(t *testing.T) {
	publicHandler, authorization, console, err := embeddedHandlerWithWorkspace(readTestResource, filepath.Join(t.TempDir(), "identity"), compositionRead)
	if err != nil {
		t.Fatal(err)
	}
	defer authorization.Close()
	defer console.Close()
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

	registration := fmt.Sprintf(`{"client_name":"Revocation HTTP test","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	status, body, _ := adminHTTP(t, client, public.URL, publicHost, "POST", "/register", registration, "", "", nil)
	if status != 201 {
		t.Fatalf("register: %d %s", status, body)
	}
	var registered struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal([]byte(body), &registered); err != nil || registered.ClientID == "" {
		t.Fatalf("client: %v", err)
	}
	// O mecanismo de workspace aceita apenas cliente que já recebeu token.
	diagnostic := authorizeClient(t, publicHandler, func(id string) error { return authorization.Approve(id, true) }, registered.ClientID, "signalspace:diagnostic")
	if diagnostic == "" || len(authorization.IssuedClients()) != 1 {
		t.Fatal("diagnostic OAuth was not completed before the grant")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("revocable test data"), 0600); err != nil {
		t.Fatal(err)
	}
	var consoleOutput bytes.Buffer
	console.handleWorkspaceCommand("workspace request "+registered.ClientID+" "+root, &consoleOutput)
	if console.pending == nil {
		t.Fatal("workspace request not created")
	}
	console.handleWorkspaceCommand("workspace approve "+console.pending.id, &consoleOutput)
	sessionMatch := regexp.MustCompile(`session=([a-f0-9]{32})`).FindStringSubmatch(consoleOutput.String())
	if len(sessionMatch) != 2 || !console.grants.AllowsClient(registered.ClientID) {
		t.Fatal("local workspace grant not active")
	}

	challenge := sha256.Sum256([]byte(readTestVerifier))
	query := url.Values{
		"client_id": {registered.ClientID}, "redirect_uri": {readTestCallback},
		"response_type": {"code"}, "scope": {"signalspace:diagnostic signalspace:workspace.read"},
		"resource": {readTestResource}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"}, "state": {"random-state-for-revocation-http"},
	}
	// Dois pedidos já criados permitem exercitar tanto aprovação posterior à
	// revogação quanto conclusão posterior à aprovação, mas já sem grant.
	requestConsent := func() (string, *http.Cookie, string) {
		t.Helper()
		got, page, cookies := adminHTTP(t, client, public.URL, publicHost, "GET", "/authorize?"+query.Encode(), "", "", "", nil)
		id := regexp.MustCompile(`approve ([A-Za-z0-9_-]{22})`).FindStringSubmatch(page)
		csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(page)
		if got != 200 || len(id) != 2 || len(csrf) != 2 || len(cookies) != 1 {
			t.Fatalf("read consent unavailable: %d %s", got, page)
		}
		return id[1], cookies[0], csrf[1]
	}
	approvedID, oauthCookie, oauthCSRF := requestConsent()
	pendingID, _, _ := requestConsent()

	status, body, bootstrapCookies := adminHTTP(t, client, private.URL, adminHost, "GET", adminPath+"/session", "", "", "", nil)
	var bootstrap struct {
		CSRF string `json:"csrf_token"`
	}
	if status != 200 || json.Unmarshal([]byte(body), &bootstrap) != nil || bootstrap.CSRF == "" {
		t.Fatalf("bootstrap: %d %s", status, body)
	}
	pairBody := fmt.Sprintf(`{"pairing_code":%q,"passphrase":"long-local-owner-passphrase"}`, pairingCode)
	status, body, ownerCookies := adminHTTP(t, client, private.URL, adminHost, "POST", adminPath+"/pair", pairBody, admin.AdminOrigin, bootstrap.CSRF, adminHTTPCookie(t, bootstrapCookies, "signalspace_admin_bootstrap"))
	var owner struct {
		CSRF string `json:"csrf_token"`
	}
	if status != 201 || json.Unmarshal([]byte(body), &owner) != nil || owner.CSRF == "" {
		t.Fatalf("pair: %d %s", status, body)
	}
	ownerCookie := adminHTTPCookie(t, ownerCookies, "signalspace_admin_session")
	approval := `{"decision":"approve","expected_version":1}`
	approvedPath := adminPath + "/requests/" + approvedID
	pendingPath := adminPath + "/requests/" + pendingID
	status, body, _ = adminHTTP(t, client, private.URL, adminHost, "POST", approvedPath+"/decision", approval, admin.AdminOrigin, owner.CSRF, ownerCookie)
	if status != 200 || !strings.Contains(body, `"status":"APPROVED"`) {
		t.Fatalf("approval before revoke failed: %d %s", status, body)
	}
	if len(authorization.IssuedClients()) != 1 {
		t.Fatal("admin approval issued a token without public completion")
	}

	console.handleWorkspaceCommand("workspace revoke "+sessionMatch[1], &consoleOutput)
	if console.grants.AllowsClient(registered.ClientID) {
		t.Fatal("workspace revoke did not invalidate the grant")
	}
	status, body, _ = adminHTTP(t, client, private.URL, adminHost, "GET", pendingPath, "", "", "", ownerCookie)
	if status != 200 || !strings.Contains(body, `"grant_status":"REVOKED"`) || !strings.Contains(body, `"status":"PENDING"`) {
		t.Fatalf("revoked grant not visible through admin GET: %d %s", status, body)
	}
	status, body, _ = adminHTTP(t, client, private.URL, adminHost, "POST", pendingPath+"/decision", approval, admin.AdminOrigin, owner.CSRF, ownerCookie)
	if status != 409 || !strings.Contains(body, "WORKSPACE_GRANT_REQUIRED") {
		t.Fatalf("revoked grant approved over HTTP: %d %s", status, body)
	}
	status, body, _ = adminHTTP(t, client, private.URL, adminHost, "GET", pendingPath, "", "", "", ownerCookie)
	if status != 200 || !strings.Contains(body, `"status":"PENDING"`) || !strings.Contains(body, `"version":1`) {
		t.Fatalf("failed approval consumed request: %d %s", status, body)
	}
	status, body, _ = adminHTTP(t, client, private.URL, adminHost, "POST", pendingPath+"/decision", `{"decision":"deny","expected_version":1}`, admin.AdminOrigin, owner.CSRF, ownerCookie)
	if status != 200 || !strings.Contains(body, `"status":"DENIED"`) {
		t.Fatalf("revoked request could not be denied: %d %s", status, body)
	}

	form := url.Values{"request": {approvedID}, "csrf": {oauthCSRF}}
	completion, err := http.NewRequest(http.MethodPost, public.URL+"/authorize/complete", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	completion.Host = publicHost
	completion.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	completion.AddCookie(oauthCookie)
	response, err := client.Do(completion)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 403 || response.Header.Get("Location") != "" {
		t.Fatalf("revoked grant completed OAuth: %d", response.StatusCode)
	}
	status, body, _ = adminHTTP(t, client, private.URL, adminHost, "GET", approvedPath, "", "", "", ownerCookie)
	if status != 200 || !strings.Contains(body, `"status":"APPROVED"`) || !strings.Contains(body, `"grant_status":"REVOKED"`) {
		t.Fatalf("revoked completion consumed prior approval: %d %s", status, body)
	}
	if status, _, _ := adminHTTP(t, client, public.URL, publicHost, "GET", "/authorize?"+query.Encode(), "", "", "", nil); status != 403 {
		t.Fatalf("new read request accepted after revoke: %d", status)
	}
	if len(authorization.IssuedClients()) != 1 {
		t.Fatal("revocation flow issued a read token")
	}
}
