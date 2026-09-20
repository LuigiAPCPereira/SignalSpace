package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
)

// O handler conclui a mutação em memória, mas segura todos os bytes da resposta
// até o socket do cliente realmente fechar. O cliente recupera por GET novo.
func TestAdminOAuthDecisionReconcilesAfterSocketDisconnect(t *testing.T) {
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

	committed := make(chan int, 1)
	disconnected := make(chan bool, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	var interceptOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	adminHandler := admin.NewServer(gate.HandlerWithRequests(authorization)).Handler
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/decision") {
			adminHandler.ServeHTTP(w, r)
			return
		}
		intercept := false
		interceptOnce.Do(func() { intercept = true })
		if !intercept {
			adminHandler.ServeHTTP(w, r)
			return
		}
		buffered := httptest.NewRecorder()
		adminHandler.ServeHTTP(buffered, r)
		committed <- buffered.Code
		select {
		case <-r.Context().Done():
			disconnected <- true
		case <-time.After(10 * time.Second):
			disconnected <- false
		}
		<-release
		for name, values := range buffered.Header() {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(buffered.Code)
		_, _ = w.Write(buffered.Body.Bytes())
	}))
	defer private.Close()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	const publicHost = "signalspace.example"
	const adminHost = "localhost:7677"
	const adminPath = "/api/admin/v1"

	registration := fmt.Sprintf(`{"client_name":"Disconnected HTTP test","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	status, body, _ := adminHTTP(t, client, public.URL, publicHost, "POST", "/register", registration, "", "", nil)
	if status != 201 {
		t.Fatalf("register: %d %s", status, body)
	}
	var registered struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal([]byte(body), &registered); err != nil || registered.ClientID == "" {
		t.Fatalf("registered client: %v", err)
	}
	challenge := sha256.Sum256([]byte(readTestVerifier))
	query := url.Values{
		"client_id": {registered.ClientID}, "redirect_uri": {readTestCallback},
		"response_type": {"code"}, "scope": {"signalspace:diagnostic"},
		"resource": {readTestResource}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"}, "state": {"random-state-for-disconnected-http"},
	}
	status, consent, _ := adminHTTP(t, client, public.URL, publicHost, "GET", "/authorize?"+query.Encode(), "", "", "", nil)
	idMatch := regexp.MustCompile(`approve ([A-Za-z0-9_-]{22})`).FindStringSubmatch(consent)
	if status != 200 || len(idMatch) != 2 {
		t.Fatalf("consent missing request: %d %s", status, consent)
	}
	id := idMatch[1]
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

	decisionPath := adminPath + "/requests/" + id + "/decision"
	decision := `{"decision":"approve","expected_version":1}`
	connection, err := net.DialTimeout("tcp", private.Listener.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := fmt.Fprintf(connection, "POST %s HTTP/1.1\r\nHost: %s\r\nOrigin: %s\r\nX-CSRF-Token: %s\r\nContent-Type: application/json\r\nCookie: %s=%s\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", decisionPath, adminHost, admin.AdminOrigin, owner.CSRF, ownerCookie.Name, ownerCookie.Value, len(decision), decision); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-committed:
		if code != 200 {
			t.Fatalf("decision did not commit before disconnect: %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server never committed the decision")
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case canceled := <-disconnected:
		if !canceled {
			t.Fatal("client connection did not cancel the server request")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not observe the socket disconnect")
	}
	releaseOnce.Do(func() { close(release) })
	status, detail, _ := adminHTTP(t, client, private.URL, adminHost, "GET", adminPath+"/requests/"+id, "", "", "", ownerCookie)
	if status != 200 || !strings.Contains(detail, `"status":"APPROVED"`) || !strings.Contains(detail, `"version":2`) {
		t.Fatalf("GET did not reconcile after actual disconnect: %d %s", status, detail)
	}
	if status, _, _ := adminHTTP(t, client, private.URL, adminHost, "POST", decisionPath, decision, admin.AdminOrigin, owner.CSRF, ownerCookie); status != 409 {
		t.Fatalf("duplicate effect accepted after disconnect: %d", status)
	}
	if len(authorization.IssuedClients()) != 0 {
		t.Fatal("administrative decision issued a token")
	}
}
