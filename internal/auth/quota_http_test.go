package auth

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

const quotaHTTPIssuer = "https://signalspace.example"

type quotaHTTPClient struct {
	base   string
	client *http.Client
}

type quotaHTTPConsent struct {
	id     string
	csrf   string
	cookie *http.Cookie
}

type quotaHTTPResponse struct {
	status  int
	header  http.Header
	body    []byte
	cookies []*http.Cookie
}

func quotaHTTPChallenge() string {
	sum := sha256.Sum256([]byte(testVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func newQuotaHTTPClient(t *testing.T, server *Server) (*quotaHTTPClient, func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/authorize/status", server.PublicStatusHandler())
	mux.Handle("/", server.Handler())
	httpServer := httptest.NewServer(mux)
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}
	cleanup := func() {
		httpServer.Close()
		if err := server.Close(); err != nil {
			t.Errorf("fechamento do emissor OAuth: %v", err)
		}
	}
	return &quotaHTTPClient{base: httpServer.URL, client: client}, cleanup
}

func (c *quotaHTTPClient) do(t *testing.T, method, path, body, contentType string, cookie *http.Cookie) quotaHTTPResponse {
	t.Helper()
	var requestBody io.Reader
	if body != "" {
		requestBody = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, c.base+path, requestBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = strings.TrimPrefix(quotaHTTPIssuer, "https://")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if method == http.MethodPost {
		request.Header.Set("Origin", quotaHTTPIssuer)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := c.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return quotaHTTPResponse{status: response.StatusCode, header: response.Header.Clone(), body: responseBody, cookies: response.Cookies()}
}

func registerQuotaHTTPClient(t *testing.T, client *quotaHTTPClient) string {
	t.Helper()
	payload := fmt.Sprintf(`{"client_name":"ChatGPT","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none","scope":%q}`, callback, diagnosticScope)
	response := client.do(t, http.MethodPost, "/register", payload, "application/json", nil)
	if response.status != http.StatusCreated {
		t.Fatalf("registro OAuth por HTTP: status=%d body=%s", response.status, response.body)
	}
	var result struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(response.body, &result); err != nil || result.ClientID == "" {
		t.Fatalf("registro OAuth sem client_id: %v", err)
	}
	return result.ClientID
}

func requestQuotaHTTPConsent(t *testing.T, client *quotaHTTPClient, clientID, state string) quotaHTTPConsent {
	t.Helper()
	sum := sha256.Sum256([]byte(testVerifier))
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {callback},
		"scope":                 {diagnosticScope},
		"resource":              {resourceURL},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	response := client.do(t, http.MethodGet, "/authorize?"+query.Encode(), "", "", nil)
	if response.status != http.StatusOK {
		t.Fatalf("authorize OAuth por HTTP: status=%d body=%s", response.status, response.body)
	}
	idMatch := regexp.MustCompile(`data-request-id="([A-Za-z0-9_-]{22})"`).FindSubmatch(response.body)
	csrfMatch := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindSubmatch(response.body)
	if len(idMatch) != 2 || len(csrfMatch) != 2 {
		t.Fatalf("consentimento HTTP sem request_id/CSRF")
	}
	var cookie *http.Cookie
	for _, candidate := range response.cookies {
		if candidate.Name == "signalspace_auth" && candidate.Value != "" {
			copy := *candidate
			cookie = &copy
			break
		}
	}
	if cookie == nil || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/authorize" {
		t.Fatal("consentimento HTTP sem cookie de sessão seguro")
	}
	return quotaHTTPConsent{id: string(idMatch[1]), csrf: string(csrfMatch[1]), cookie: cookie}
}

func postQuotaHTTPComplete(t *testing.T, client *quotaHTTPClient, consent quotaHTTPConsent) quotaHTTPResponse {
	t.Helper()
	form := url.Values{"request": {consent.id}, "csrf": {consent.csrf}}
	return client.do(t, http.MethodPost, "/authorize/complete", form.Encode(), "application/x-www-form-urlencoded", consent.cookie)
}

func readQuotaHTTPStatus(t *testing.T, client *quotaHTTPClient, consent quotaHTTPConsent) (quotaHTTPResponse, map[string]string) {
	t.Helper()
	response := client.do(t, http.MethodGet, "/authorize/status?request_id="+url.QueryEscape(consent.id), "", "", consent.cookie)
	var result map[string]string
	if response.status == http.StatusOK {
		if err := json.Unmarshal(response.body, &result); err != nil {
			t.Fatalf("status público HTTP inválido: %v", err)
		}
	}
	return response, result
}

func assertQuotaHTTPNoCredentials(t *testing.T, s *Server) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.codes) != 0 || len(s.issued) != 0 {
		t.Fatalf("fila OAuth criou credencial indevida: codes=%d issued=%d", len(s.codes), len(s.issued))
	}
}

func TestOAuthPendingQueueSaturationAndHTTPRecovery(t *testing.T) {
	server, err := New(Config{
		ResourceURL: resourceURL,
		Issuer:      quotaHTTPIssuer,
		Scope:       diagnosticScope,
		StateDir:    filepath.Join(t.TempDir(), "identity"),
	})
	if err != nil {
		t.Fatal(err)
	}
	client, cleanup := newQuotaHTTPClient(t, server)
	t.Cleanup(cleanup)
	clientID := registerQuotaHTTPClient(t, client)

	consents := make([]quotaHTTPConsent, 0, maxPending)
	for i := 0; i < maxPending; i++ {
		consent := requestQuotaHTTPConsent(t, client, clientID, fmt.Sprintf("queue-saturation-state-%02d-20260922", i))
		consents = append(consents, consent)
	}
	server.mu.Lock()
	if len(server.pending) != maxPending {
		server.mu.Unlock()
		t.Fatalf("fila não atingiu maxPending: got=%d want=%d", len(server.pending), maxPending)
	}
	for _, consent := range consents {
		pending, ok := server.pending[consent.id]
		if !ok || pending.Scope != diagnosticScope || pending.Version != 1 || pending.Approved || pending.Denied {
			server.mu.Unlock()
			t.Fatalf("pedido aceito não permaneceu PENDING: id=%s found=%t state=%+v", consent.id, ok, pending)
		}
	}
	server.mu.Unlock()
	assertQuotaHTTPNoCredentials(t, server)

	overLimit := client.do(t, http.MethodGet, "/authorize?response_type=code&client_id="+url.QueryEscape(clientID)+"&redirect_uri="+url.QueryEscape(callback)+"&scope="+url.QueryEscape(diagnosticScope)+"&resource="+url.QueryEscape(resourceURL)+"&code_challenge="+url.QueryEscape(quotaHTTPChallenge())+"&code_challenge_method=S256&state=queue-saturation-over-limit-20260922", "", "", nil)
	if overLimit.status != http.StatusServiceUnavailable || !strings.Contains(string(overLimit.body), "temporarily_unavailable") || len(overLimit.cookies) != 0 {
		t.Fatalf("saturação não recusou de forma fechada: status=%d cookies=%d body=%s", overLimit.status, len(overLimit.cookies), overLimit.body)
	}
	server.mu.Lock()
	if len(server.pending) != maxPending {
		server.mu.Unlock()
		t.Fatalf("tentativa recusada alterou a fila: got=%d", len(server.pending))
	}
	server.mu.Unlock()
	assertQuotaHTTPNoCredentials(t, server)

	if err := server.DecideTerminal(consents[0].id, false); err != nil {
		t.Fatal(err)
	}
	snapshot, err := server.GetRequestSnapshot(consents[0].id)
	if err != nil || snapshot.Status != "DENIED" || snapshot.Version != 2 {
		t.Fatalf("decisão negativa inesperada antes da conclusão HTTP: snapshot=%+v err=%v", snapshot, err)
	}
	server.mu.Lock()
	_, stillPending := server.pending[consents[0].id]
	queueAfterDecision := len(server.pending)
	server.mu.Unlock()
	if !stillPending || queueAfterDecision != maxPending {
		t.Fatalf("decisão negativa liberou a vaga antes da conclusão: pending=%t count=%d", stillPending, queueAfterDecision)
	}
	stillFull := client.do(t, http.MethodGet, "/authorize?response_type=code&client_id="+url.QueryEscape(clientID)+"&redirect_uri="+url.QueryEscape(callback)+"&scope="+url.QueryEscape(diagnosticScope)+"&resource="+url.QueryEscape(resourceURL)+"&code_challenge="+url.QueryEscape(quotaHTTPChallenge())+"&code_challenge_method=S256&state=queue-saturation-still-full-20260922", "", "", nil)
	if stillFull.status != http.StatusServiceUnavailable || len(stillFull.cookies) != 0 {
		t.Fatalf("DecideTerminal negativo liberou vaga indevidamente: status=%d cookies=%d", stillFull.status, len(stillFull.cookies))
	}
	if got := postQuotaHTTPComplete(t, client, consents[0]); got.status != http.StatusForbidden || strings.Contains(string(got.body), "code") || got.header.Get("Location") != "" {
		t.Fatalf("conclusão negada não liberou a vaga sem credencial: status=%d location=%q body=%s", got.status, got.header.Get("Location"), got.body)
	}
	server.mu.Lock()
	if len(server.pending) != maxPending-1 {
		server.mu.Unlock()
		t.Fatalf("conclusão negada não liberou a vaga: got=%d", len(server.pending))
	}
	server.mu.Unlock()
	assertQuotaHTTPNoCredentials(t, server)

	recovered := requestQuotaHTTPConsent(t, client, clientID, "queue-saturation-recovered-20260922")
	if recovered.id == consents[0].id {
		t.Fatal("nova solicitação reutilizou o request_id encerrado")
	}
	server.mu.Lock()
	if len(server.pending) != maxPending {
		server.mu.Unlock()
		t.Fatalf("vaga não foi reutilizada após conclusão negada: got=%d", len(server.pending))
	}
	for _, consent := range consents[1:] {
		pending, ok := server.pending[consent.id]
		if !ok || pending.Version != 1 || pending.Approved || pending.Denied || pending.Scope != diagnosticScope {
			server.mu.Unlock()
			t.Fatalf("pedido não escolhido foi alterado: id=%s found=%t state=%+v", consent.id, ok, pending)
		}
	}
	server.mu.Unlock()
	assertQuotaHTTPNoCredentials(t, server)
}

func TestOAuthPublicStatusQuotaResetsWithoutRenewingRequest(t *testing.T) {
	server, err := New(Config{
		ResourceURL: resourceURL,
		Issuer:      quotaHTTPIssuer,
		Scope:       diagnosticScope,
		StateDir:    filepath.Join(t.TempDir(), "identity"),
	})
	if err != nil {
		t.Fatal(err)
	}
	client, cleanup := newQuotaHTTPClient(t, server)
	t.Cleanup(cleanup)
	clientID := registerQuotaHTTPClient(t, client)
	consent := requestQuotaHTTPConsent(t, client, clientID, "public-status-quota-20260922")

	first, firstValue := readQuotaHTTPStatus(t, client, consent)
	if first.status != http.StatusOK || firstValue["status"] != "PENDING" || firstValue["expires_at"] == "" || first.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("status inicial inesperado: status=%d state=%q expires=%q cache=%q", first.status, firstValue["status"], firstValue["expires_at"], first.header.Get("Cache-Control"))
	}
	expiresAt := firstValue["expires_at"]
	for i := 1; i < 120; i++ {
		response, value := readQuotaHTTPStatus(t, client, consent)
		if response.status != http.StatusOK || value["status"] != "PENDING" || value["expires_at"] != expiresAt || response.header.Get("Cache-Control") != "no-store" {
			t.Fatalf("polling legítimo %d inesperado: status=%d value=%v", i, response.status, value)
		}
	}
	server.mu.Lock()
	quotaBeforeOverLimit := server.quotas[publicStatusPath]
	pendingBeforeReset := server.pending[consent.id]
	quotasBeforeReset := cloneQuotaHTTPQuotas(server.quotas)
	server.mu.Unlock()
	if quotaBeforeOverLimit.Count != 120 || pendingBeforeReset.Approved || pendingBeforeReset.Denied {
		t.Fatalf("estado antes do excesso inesperado: quota=%+v pending=%+v", quotaBeforeOverLimit, pendingBeforeReset)
	}
	overLimit, value := readQuotaHTTPStatus(t, client, consent)
	if overLimit.status != http.StatusTooManyRequests || !strings.Contains(string(overLimit.body), "slow_down") || overLimit.header.Get("Retry-After") == "" || overLimit.header.Get("Cache-Control") != "no-store" || value != nil {
		t.Fatalf("cota pública não recusou de forma fechada: status=%d retry=%q cache=%q body=%s", overLimit.status, overLimit.header.Get("Retry-After"), overLimit.header.Get("Cache-Control"), overLimit.body)
	}
	assertQuotaHTTPNoCredentials(t, server)

	server.mu.Lock()
	quota := server.quotas[publicStatusPath]
	quota.Started = time.Now().Add(-quotaWindow)
	server.quotas[publicStatusPath] = quota
	server.mu.Unlock()
	afterReset, afterResetValue := readQuotaHTTPStatus(t, client, consent)
	if afterReset.status != http.StatusOK || afterResetValue["status"] != "PENDING" || afterResetValue["expires_at"] != expiresAt || afterReset.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("reset da cota não preservou o pedido: status=%d value=%v", afterReset.status, afterResetValue)
	}
	server.mu.Lock()
	quotaAfterReset := server.quotas[publicStatusPath]
	pendingAfterReset := server.pending[consent.id]
	quotasAfterReset := cloneQuotaHTTPQuotas(server.quotas)
	server.mu.Unlock()
	if quotaAfterReset.Count != 1 || pendingAfterReset.Expires.UTC().Format(time.RFC3339Nano) != expiresAt || pendingAfterReset.Approved || pendingAfterReset.Denied {
		t.Fatalf("reset da cota alterou o pedido ou a contagem: quota=%+v pending=%+v", quotaAfterReset, pendingAfterReset)
	}
	for path, before := range quotasBeforeReset {
		if path == publicStatusPath {
			continue
		}
		if after, ok := quotasAfterReset[path]; !ok || after != before {
			t.Fatalf("reset do status alterou a cota de outra rota: path=%q before=%+v after=%+v", path, before, after)
		}
	}
	if pendingAfterReset.Expires.UTC().Format(time.RFC3339Nano) != expiresAt {
		t.Fatalf("polling renovou expires_at: got=%s want=%s", pendingAfterReset.Expires.UTC().Format(time.RFC3339Nano), expiresAt)
	}
	assertQuotaHTTPNoCredentials(t, server)
}

func cloneQuotaHTTPQuotas(source map[string]requestQuota) map[string]requestQuota {
	clone := make(map[string]requestQuota, len(source))
	for path, quota := range source {
		clone[path] = quota
	}
	return clone
}
