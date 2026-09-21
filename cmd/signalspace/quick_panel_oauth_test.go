package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/tunnel"
)

// Exercita a composição real do Quick, não um handler criado fora do processo.
// Somente o processo cloudflared e a verificação HTTPS são simulados; os dois
// listeners, OAuth, Gate, cookies e decisões são reais em HTTP loopback.
func TestQuickPanelOAuthDecisionAcrossActualListeners(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	const issuer = "https://panel-oauth-test.trycloudflare.com"
	const resource = issuer + "/mcp"
	const publicHost = "panel-oauth-test.trycloudflare.com"
	const adminHost = "localhost:7677"
	const adminBase = "/api/admin/v1"

	path := filepath.Join(t.TempDir(), "cloudflared")
	script := "#!/bin/sh\n[ \"$1\" = tunnel ] && [ \"$2\" = --url ] && [ \"$3\" = http://127.0.0.1:7676 ] || exit 2\necho '" + issuer + "' >&2\nexec sleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output synchronizedBuffer
	verified := make(chan struct{})
	finished := make(chan error, 1)
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport, Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	go func() {
		finished <- runQuickWithOptions(ctx, strings.NewReader("PUBLICAR PAINEL\n"), &output, tunnel.Start, func(_ context.Context, got string) (mcp.TransportReport, error) {
			if got != resource {
				return mcp.TransportReport{}, fmt.Errorf("unexpected public resource: %s", got)
			}
			close(verified)
			return mcp.TransportReport{ResourceURL: got}, nil
		}, false, true)
	}()
	select {
	case <-verified:
	case err := <-finished:
		t.Fatalf("Quick stopped before HTTPS verifier: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("Quick never reached HTTPS verifier")
	}
	deadline := time.After(3 * time.Second)
	for !strings.Contains(output.String(), "Cole no ChatGPT Web") {
		select {
		case err := <-finished:
			t.Fatalf("Quick stopped before verified URL: %v", err)
		case <-deadline:
			t.Fatal("Quick never announced verified URL")
		case <-time.After(10 * time.Millisecond):
		}
	}
	match := regexp.MustCompile(`Código de pareamento desta instância \(somente neste terminal, expira em 5 minutos\): ([A-Za-z0-9_-]+)`).FindStringSubmatch(output.String())
	if len(match) != 2 {
		t.Fatal("pairing code missing from terminal")
	}

	status, bootstrapJSON, bootstrapCookies := adminHTTP(t, client, "http://"+admin.AdminAddress, adminHost, "GET", adminBase+"/session", "", "", "", nil)
	var bootstrap struct {
		CSRF string `json:"csrf_token"`
	}
	if status != 200 || json.Unmarshal([]byte(bootstrapJSON), &bootstrap) != nil || bootstrap.CSRF == "" {
		t.Fatalf("bootstrap failed: %d", status)
	}
	pairing := fmt.Sprintf(`{"pairing_code":%q,"passphrase":"long-local-owner-passphrase"}`, match[1])
	status, sessionJSON, pairedCookies := adminHTTP(t, client, "http://"+admin.AdminAddress, adminHost, "POST", adminBase+"/pair", pairing, admin.AdminOrigin, bootstrap.CSRF, adminHTTPCookie(t, bootstrapCookies, "signalspace_admin_bootstrap"))
	var owner struct {
		CSRF string `json:"csrf_token"`
	}
	if status != 201 || json.Unmarshal([]byte(sessionJSON), &owner) != nil || owner.CSRF == "" {
		t.Fatalf("owner pairing failed: %d", status)
	}
	ownerCookie := adminHTTPCookie(t, pairedCookies, "signalspace_admin_session")

	registration := fmt.Sprintf(`{"client_name":"Quick listener test","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	status, registeredJSON, _ := adminHTTP(t, client, "http://"+admin.PublicAddress, publicHost, "POST", "/register", registration, "", "", nil)
	var registered struct {
		ClientID string `json:"client_id"`
	}
	if status != 201 || json.Unmarshal([]byte(registeredJSON), &registered) != nil || registered.ClientID == "" {
		t.Fatalf("public registration failed: %d", status)
	}
	challenge := sha256.Sum256([]byte(readTestVerifier))
	query := url.Values{
		"client_id": {registered.ClientID}, "redirect_uri": {readTestCallback},
		"response_type": {"code"}, "scope": {"signalspace:diagnostic"},
		"resource": {resource}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"}, "state": {"long-random-state-for-quick-panel-test"},
	}
	status, consent, oauthCookies := adminHTTP(t, client, "http://"+admin.PublicAddress, publicHost, "GET", "/authorize?"+query.Encode(), "", "", "", nil)
	csrfMatch := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(consent)
	if status != 200 || len(csrfMatch) != 2 || len(oauthCookies) != 1 {
		t.Fatalf("public OAuth consent failed: %d", status)
	}

	status, listJSON, _ := adminHTTP(t, client, "http://"+admin.AdminAddress, adminHost, "GET", adminBase+"/requests", "", "", "", ownerCookie)
	var list struct {
		Requests []struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
			Status  string `json:"status"`
		} `json:"requests"`
	}
	if status != 200 || json.Unmarshal([]byte(listJSON), &list) != nil || len(list.Requests) != 1 || list.Requests[0].Status != "PENDING" || list.Requests[0].Version != 1 || list.Requests[0].ID == "" {
		t.Fatalf("pending OAuth request missing from actual Quick queue: %d", status)
	}
	id := list.Requests[0].ID
	publicStatus := "/authorize/status?request_id=" + url.QueryEscape(id)
	if status, _, _ := adminHTTP(t, client, "http://"+admin.PublicAddress, publicHost, "GET", publicStatus, "", "", "", nil); status != 403 {
		t.Fatalf("public status leaked without OAuth cookie: %d", status)
	}
	status, publicJSON, _ := adminHTTP(t, client, "http://"+admin.PublicAddress, publicHost, "GET", publicStatus, "", "", "", oauthCookies[0])
	if status != 200 || !strings.Contains(publicJSON, `"status":"PENDING"`) {
		t.Fatalf("own public status not pending: %d", status)
	}
	decisionPath := adminBase + "/requests/" + id + "/decision"
	decision := `{"decision":"approve","expected_version":1}`
	status, decisionJSON, _ := adminHTTP(t, client, "http://"+admin.AdminAddress, adminHost, "POST", decisionPath, decision, admin.AdminOrigin, owner.CSRF, ownerCookie)
	if status != 200 || !strings.Contains(decisionJSON, `"status":"APPROVED"`) || !strings.Contains(decisionJSON, `"version":2`) {
		t.Fatalf("admin decision did not commit: %d", status)
	}
	status, publicJSON, _ = adminHTTP(t, client, "http://"+admin.PublicAddress, publicHost, "GET", publicStatus, "", "", "", oauthCookies[0])
	if status != 200 || !strings.Contains(publicJSON, `"status":"APPROVED"`) {
		t.Fatalf("public origin did not see own approval: %d", status)
	}
	form := url.Values{"request": {id}, "csrf": {csrfMatch[1]}}
	complete, err := http.NewRequest("POST", "http://"+admin.PublicAddress+"/authorize/complete", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	complete.Host = publicHost
	complete.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	complete.AddCookie(oauthCookies[0])
	response, err := client.Do(complete)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	redirect, parseErr := url.Parse(response.Header.Get("Location"))
	if response.StatusCode != 303 || parseErr != nil || redirect.Host != "chatgpt.com" || redirect.Query().Get("code") == "" {
		t.Fatalf("public completion failed: status=%d redirect_valid=%t", response.StatusCode, parseErr == nil)
	}
	status, detail, _ := adminHTTP(t, client, "http://"+admin.AdminAddress, adminHost, "GET", adminBase+"/requests/"+id, "", "", "", ownerCookie)
	if status != 200 || !strings.Contains(detail, `"status":"COMPLETED"`) {
		t.Fatalf("admin did not retain completed request: %d", status)
	}

	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("Quick shutdown failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Quick did not shut down after OAuth decision")
	}
	for _, address := range []string{admin.PublicAddress, admin.AdminAddress} {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			t.Fatalf("Quick leaked listener after decision: %v", err)
		}
		_ = listener.Close()
	}
}
