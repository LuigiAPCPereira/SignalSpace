package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	realExpiryIssuer   = "https://localhost:7676"
	realExpiryResource = realExpiryIssuer + "/mcp"
	realExpiryCallback = callback
)

type realExpiryHTTPClient struct {
	base   string
	client *http.Client
}

func newRealExpiryHTTPClient(base string) *realExpiryHTTPClient {
	return &realExpiryHTTPClient{
		base: base,
		client: &http.Client{
			Transport: &http.Transport{Proxy: nil},
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: 10 * time.Second,
		},
	}
}

func (c *realExpiryHTTPClient) do(t *testing.T, method, path, body, contentType string, cookie *http.Cookie) (int, http.Header, []byte, []*http.Cookie, error) {
	t.Helper()
	var requestBody io.Reader
	if body != "" {
		requestBody = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, c.base+path, requestBody)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	request.Host = "localhost:7676"
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if method == http.MethodPost {
		request.Header.Set("Origin", realExpiryIssuer)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	return response.StatusCode, response.Header.Clone(), responseBody, response.Cookies(), err
}

func startRealExpiryHTTPServer(t *testing.T, server *Server) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listener OAuth loopback efêmero: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/authorize/status", server.PublicStatusHandler())
	mux.Handle("/", server.Handler())
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	served := make(chan error, 1)
	go func() { served <- httpServer.Serve(listener) }()
	address := listener.Addr().String()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			t.Errorf("shutdown do emissor OAuth: %v", err)
		}
		if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("emissor OAuth terminou com erro: %v", err)
		}
		if err := server.Close(); err != nil {
			t.Errorf("fechamento do emissor OAuth: %v", err)
		}
		probe, err := net.Listen("tcp4", address)
		if err != nil {
			t.Errorf("listener OAuth residual em %s: %v", address, err)
			return
		}
		_ = probe.Close()
	})
	return "http://" + address
}

func realExpiryRegisterPayload(t *testing.T) string {
	t.Helper()
	payload := map[string]any{
		"client_name":                "ChatGPT",
		"redirect_uris":              []string{realExpiryCallback},
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"scope":                      diagnosticScope,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func registerRealExpiryClient(t *testing.T, client *realExpiryHTTPClient) string {
	t.Helper()
	status, _, body, _, err := client.do(t, http.MethodPost, "/register", realExpiryRegisterPayload(t), "application/json", nil)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("registro OAuth real: status=%d err=%v", status, err)
	}
	var response struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.ClientID == "" {
		t.Fatalf("registro OAuth sem client_id: err=%v", err)
	}
	return response.ClientID
}

func requestRealExpiryConsent(t *testing.T, client *realExpiryHTTPClient, clientID string) (RequestInfo, *http.Cookie, string) {
	t.Helper()
	sum := sha256.Sum256([]byte(testVerifier))
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {realExpiryCallback},
		"scope":                 {diagnosticScope},
		"resource":              {realExpiryResource},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"state":                 {"real-http-expiry-state-2026"},
	}
	status, _, body, cookies, err := client.do(t, http.MethodGet, "/authorize?"+query.Encode(), "", "", nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("authorize OAuth real: status=%d err=%v", status, err)
	}
	var csrf string
	match := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(string(body))
	if len(match) == 2 {
		csrf = match[1]
	}
	if csrf == "" {
		t.Fatal("consentimento OAuth não retornou CSRF")
	}
	var cookie *http.Cookie
	for _, candidate := range cookies {
		if candidate.Name == "signalspace_auth" && candidate.Value != "" {
			copy := *candidate
			cookie = &copy
			break
		}
	}
	if cookie == nil || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/authorize" {
		t.Fatal("consentimento OAuth não retornou cookie de sessão válido")
	}
	return RequestInfo{ID: matchRequestID(t, body), ClientID: clientID}, cookie, csrf
}

func matchRequestID(t *testing.T, body []byte) string {
	t.Helper()
	match := regexp.MustCompile(`data-request-id="([A-Za-z0-9_-]{22})"`).FindSubmatch(body)
	if len(match) != 2 {
		t.Fatal("consentimento OAuth não retornou request_id opaco")
	}
	return string(match[1])
}

func waitRealExpiryRequest(t *testing.T, events <-chan RequestInfo, clientID, requestID string) RequestInfo {
	t.Helper()
	select {
	case event := <-events:
		if event.ID != requestID || event.ClientID != clientID || event.Scope != diagnosticScope {
			t.Fatalf("evento OAuth inesperado: id_match=%t client_match=%t scope=%q", event.ID == requestID, event.ClientID == clientID, event.Scope)
		}
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("emissor OAuth não observou pedido")
		return RequestInfo{}
	}
}

type realExpiryPublicStatus struct {
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
}

func readRealExpiryStatus(t *testing.T, client *realExpiryHTTPClient, requestID string, cookie *http.Cookie) (int, http.Header, []byte, realExpiryPublicStatus) {
	t.Helper()
	status, headers, body, _, err := client.do(t, http.MethodGet, "/authorize/status?request_id="+url.QueryEscape(requestID), "", "", cookie)
	if err != nil {
		t.Fatal(err)
	}
	var value realExpiryPublicStatus
	if status == http.StatusOK {
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatalf("status público inválido: %v", err)
		}
	}
	return status, headers, body, value
}

func assertNoRealExpirySecrets(t *testing.T, body []byte, requestID string) {
	t.Helper()
	for _, secret := range []string{"code", "access_token", "signalspace_auth", requestID} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("status público expôs dado privado %q", secret)
		}
	}
}

func TestPublicStatusRetainsRealExpiredRequestOverHTTP(t *testing.T) {
	events := make(chan RequestInfo, 1)
	server, err := New(Config{
		ResourceURL: realExpiryResource,
		Issuer:      realExpiryIssuer,
		Scope:       diagnosticScope,
		StateDir:    filepath.Join(t.TempDir(), "identity"),
		OnRequest:   func(event RequestInfo) { events <- event },
	})
	if err != nil {
		t.Fatal(err)
	}
	base := startRealExpiryHTTPServer(t, server)
	client := newRealExpiryHTTPClient(base)
	clientID := registerRealExpiryClient(t, client)
	consent, cookie, csrf := requestRealExpiryConsent(t, client, clientID)
	event := waitRealExpiryRequest(t, events, clientID, consent.ID)
	if event.Redirect == "" || event.Client == "" {
		t.Fatal("evento OAuth não identificou o pedido real")
	}

	statusCode, headers, body, pending := readRealExpiryStatus(t, client, consent.ID, cookie)
	if statusCode != http.StatusOK || pending.Status != "PENDING" || pending.ExpiresAt == "" || headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("status pending inesperado: status=%d state=%q expires=%q cache=%q", statusCode, pending.Status, pending.ExpiresAt, headers.Get("Cache-Control"))
	}
	assertNoRealExpirySecrets(t, body, consent.ID)

	server.mu.Lock()
	item, ok := server.pending[consent.ID]
	if !ok {
		server.mu.Unlock()
		t.Fatal("pedido real não está pendente antes da expiração")
	}
	item.Expires = time.Now().Add(-time.Second)
	server.pending[consent.ID] = item
	server.mu.Unlock()

	statusCode, _, body, expired := readRealExpiryStatus(t, client, consent.ID, cookie)
	if statusCode != http.StatusOK || expired.Status != "EXPIRED" || expired.ExpiresAt == "" {
		t.Fatalf("status expired inesperado: status=%d state=%q expires=%q", statusCode, expired.Status, expired.ExpiresAt)
	}
	assertNoRealExpirySecrets(t, body, consent.ID)
	server.mu.Lock()
	record, retained := server.terminal[consent.ID]
	_, stillPending := server.pending[consent.ID]
	server.mu.Unlock()
	if !retained || stillPending || record.snapshot.Status != "EXPIRED" {
		t.Fatal("expiração não criou tombstone real ou manteve pending")
	}
	retainUntil := record.retainUntil

	statusCode, _, body, retainedExpired := readRealExpiryStatus(t, client, consent.ID, cookie)
	if statusCode != http.StatusOK || retainedExpired.Status != "EXPIRED" || retainedExpired.ExpiresAt != expired.ExpiresAt {
		t.Fatalf("polling alterou tombstone: status=%d state=%q expires=%q", statusCode, retainedExpired.Status, retainedExpired.ExpiresAt)
	}
	assertNoRealExpirySecrets(t, body, consent.ID)
	server.mu.Lock()
	recordAfterPoll := server.terminal[consent.ID]
	server.mu.Unlock()
	if !recordAfterPoll.retainUntil.Equal(retainUntil) {
		t.Fatal("polling renovou a retenção do tombstone")
	}

	form := url.Values{"request": {consent.ID}, "csrf": {csrf}}
	statusCode, _, body, _, err = client.do(t, http.MethodPost, "/authorize/complete", form.Encode(), "application/x-www-form-urlencoded", cookie)
	if err != nil {
		t.Fatal(err)
	}
	if statusCode == http.StatusSeeOther {
		t.Fatal("conclusão tardia redirecionou com código OAuth")
	}
	if statusCode != http.StatusForbidden {
		t.Fatalf("conclusão tardia teve status inesperado: %d", statusCode)
	}
	assertNoRealExpirySecrets(t, body, consent.ID)
	server.mu.Lock()
	codes := len(server.codes)
	server.mu.Unlock()
	if codes != 0 || len(server.IssuedClients()) != 0 {
		t.Fatalf("conclusão expirada criou efeitos: codes=%d issued_clients=%d", codes, len(server.IssuedClients()))
	}
	if err := server.DecideVersioned(consent.ID, 1, "approve"); !errors.Is(err, ErrOAuthRequestExpired) {
		t.Fatalf("decisão tardia não retornou expiração: %v", err)
	}

	server.mu.Lock()
	record = server.terminal[consent.ID]
	record.retainUntil = time.Now().Add(-time.Second)
	server.terminal[consent.ID] = record
	server.mu.Unlock()
	statusCode, _, body, _ = readRealExpiryStatus(t, client, consent.ID, cookie)
	if statusCode != http.StatusNotFound || string(body) != "{\"error\":\"not_found\"}\n" {
		t.Fatalf("tombstone expirado não foi removido: status=%d body=%q", statusCode, body)
	}
	if err := server.DecideVersioned(consent.ID, 1, "approve"); !errors.Is(err, ErrOAuthRequestNotFound) {
		t.Fatalf("decisão após limpeza não retornou not found: %v", err)
	}
	statusCode, _, body, _ = readRealExpiryStatus(t, client, consent.ID, cookie)
	if statusCode != http.StatusNotFound || string(body) != "{\"error\":\"not_found\"}\n" {
		t.Fatalf("consulta ressuscitou tombstone: status=%d body=%q", statusCode, body)
	}
	assertNoRealExpirySecrets(t, body, consent.ID)
}
