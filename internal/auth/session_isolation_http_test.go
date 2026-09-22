package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

type group8HTTPStatus struct {
	status    string
	expiresAt string
}

func registerGroup8HTTPClient(t *testing.T, client *quotaHTTPClient, name string) string {
	t.Helper()
	payload := fmt.Sprintf(`{"client_name":%q,"redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none","scope":%q}`, name, callback, diagnosticScope)
	response := client.do(t, http.MethodPost, "/register", payload, "application/json", nil)
	if response.status != http.StatusCreated {
		t.Fatalf("registro %s: status=%d", name, response.status)
	}
	var result struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(response.body, &result); err != nil || result.ClientID == "" {
		t.Fatalf("registro %s sem client_id: erro=%v", name, err)
	}
	return result.ClientID
}

func readGroup8HTTPStatus(t *testing.T, client *quotaHTTPClient, id string, cookie *http.Cookie) (quotaHTTPResponse, group8HTTPStatus) {
	t.Helper()
	response := client.do(t, http.MethodGet, "/authorize/status?request_id="+url.QueryEscape(id), "", "", cookie)
	var value group8HTTPStatus
	if response.status == http.StatusOK {
		var body struct {
			Status    string `json:"status"`
			ExpiresAt string `json:"expires_at"`
		}
		if err := json.Unmarshal(response.body, &body); err != nil {
			t.Fatalf("status público inválido: %v", err)
		}
		value = group8HTTPStatus{status: body.Status, expiresAt: body.ExpiresAt}
	}
	return response, value
}

func completeGroup8HTTP(t *testing.T, client *quotaHTTPClient, id, csrf string, cookie *http.Cookie) quotaHTTPResponse {
	t.Helper()
	form := url.Values{"request": {id}, "csrf": {csrf}}
	return client.do(t, http.MethodPost, "/authorize/complete", form.Encode(), "application/x-www-form-urlencoded", cookie)
}

func assertGroup8PublicResponse(t *testing.T, response quotaHTTPResponse, expectedStatus string, expectedExpiresAt string) group8HTTPStatus {
	t.Helper()
	if response.status != http.StatusOK || response.header.Get("Cache-Control") != "no-store" || len(response.cookies) != 0 {
		t.Fatalf("status público inseguro: http=%d cache=%q cookies=%d", response.status, response.header.Get("Cache-Control"), len(response.cookies))
	}
	var body struct {
		Status    string `json:"status"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(response.body, &body); err != nil {
		t.Fatalf("status público não é JSON: %v", err)
	}
	if body.Status != expectedStatus || (expectedExpiresAt != "" && body.ExpiresAt != expectedExpiresAt) {
		t.Fatalf("status público inesperado: status=%q expires_presente=%t", body.Status, body.ExpiresAt != "")
	}
	for _, secret := range []string{"client_id", "redirect_uri", "csrf", "access_token", "signalspace_auth"} {
		if strings.Contains(string(response.body), secret) {
			t.Fatalf("status público expôs campo privado %q", secret)
		}
	}
	return group8HTTPStatus{status: body.Status, expiresAt: body.ExpiresAt}
}

func assertGroup8NoCredentials(t *testing.T, server *Server) {
	t.Helper()
	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.codes) != 0 || len(server.issued) != 0 {
		t.Fatalf("instância criou credencial antes da conclusão legítima: codes=%d issued=%d", len(server.codes), len(server.issued))
	}
}

func TestOAuthHTTPSessionIsolationAndSingleLegitimateCompletion(t *testing.T) {
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
	clientA := registerGroup8HTTPClient(t, client, "Fixture A")
	clientB := registerGroup8HTTPClient(t, client, "Fixture B")
	consentA := requestQuotaHTTPConsent(t, client, clientA, "session-isolation-a-20260922")
	consentB := requestQuotaHTTPConsent(t, client, clientB, "session-isolation-b-20260922")
	if consentA.id == consentB.id || consentA.cookie.Value == consentB.cookie.Value || consentA.csrf == consentB.csrf {
		t.Fatal("pedidos A/B não receberam identificadores, cookies e CSRF independentes")
	}
	assertGroup8NoCredentials(t, server)

	statusA, valueA := readGroup8HTTPStatus(t, client, consentA.id, consentA.cookie)
	initialA := assertGroup8PublicResponse(t, statusA, "PENDING", "")
	statusB, valueB := readGroup8HTTPStatus(t, client, consentB.id, consentB.cookie)
	initialB := assertGroup8PublicResponse(t, statusB, "PENDING", "")
	if valueA != initialA || valueB != initialB {
		t.Fatal("status inicial divergiu da resposta validada")
	}
	crossA, _ := readGroup8HTTPStatus(t, client, consentA.id, consentB.cookie)
	if crossA.status != http.StatusForbidden || strings.Contains(string(crossA.body), "code") || strings.Contains(string(crossA.body), "token") {
		t.Fatalf("cookie B consultou pedido A: http=%d", crossA.status)
	}
	crossB, _ := readGroup8HTTPStatus(t, client, consentB.id, consentA.cookie)
	if crossB.status != http.StatusForbidden {
		t.Fatalf("cookie A consultou pedido B: http=%d", crossB.status)
	}
	anonymousA, _ := readGroup8HTTPStatus(t, client, consentA.id, nil)
	if anonymousA.status != http.StatusForbidden {
		t.Fatalf("pedido A ficou consultável sem cookie: http=%d", anonymousA.status)
	}

	if err := server.DecideTerminal(consentA.id, true); err != nil {
		t.Fatal(err)
	}
	approvedAResponse, approvedA := readGroup8HTTPStatus(t, client, consentA.id, consentA.cookie)
	assertGroup8PublicResponse(t, approvedAResponse, "APPROVED", initialA.expiresAt)
	pendingBResponse, pendingB := readGroup8HTTPStatus(t, client, consentB.id, consentB.cookie)
	assertGroup8PublicResponse(t, pendingBResponse, "PENDING", initialB.expiresAt)
	if approvedA.status != "APPROVED" || pendingB.status != "PENDING" {
		t.Fatal("aprovação de A contaminou o estado público de B")
	}
	approvedAcrossCookie, _ := readGroup8HTTPStatus(t, client, consentA.id, consentB.cookie)
	if approvedAcrossCookie.status != http.StatusForbidden {
		t.Fatalf("cookie B acessou A depois da aprovação: http=%d", approvedAcrossCookie.status)
	}
	assertGroup8NoCredentials(t, server)

	invalidCompletions := []struct {
		name   string
		csrf   string
		cookie *http.Cookie
	}{
		{name: "cookie cruzado", csrf: consentA.csrf, cookie: consentB.cookie},
		{name: "cookie ausente", csrf: consentA.csrf},
		{name: "csrf cruzado", csrf: consentB.csrf, cookie: consentA.cookie},
	}
	for _, attempt := range invalidCompletions {
		response := completeGroup8HTTP(t, client, consentA.id, attempt.csrf, attempt.cookie)
		if response.status != http.StatusForbidden || response.header.Get("Location") != "" || strings.Contains(string(response.body), "code") {
			t.Fatalf("conclusão inválida %s não foi negada sem redirect: http=%d location=%t", attempt.name, response.status, response.header.Get("Location") != "")
		}
		approved, snapshot := readGroup8HTTPStatus(t, client, consentA.id, consentA.cookie)
		assertGroup8PublicResponse(t, approved, "APPROVED", initialA.expiresAt)
		if snapshot.status != "APPROVED" {
			t.Fatalf("conclusão inválida %s consumiu A", attempt.name)
		}
		stillB, bSnapshot := readGroup8HTTPStatus(t, client, consentB.id, consentB.cookie)
		assertGroup8PublicResponse(t, stillB, "PENDING", initialB.expiresAt)
		if bSnapshot.status != "PENDING" {
			t.Fatalf("conclusão inválida %s alterou B", attempt.name)
		}
	}
	assertGroup8NoCredentials(t, server)

	legitimate := completeGroup8HTTP(t, client, consentA.id, consentA.csrf, consentA.cookie)
	if legitimate.status != http.StatusSeeOther || legitimate.header.Get("Location") == "" {
		t.Fatalf("conclusão legítima inesperada: http=%d location_presente=%t", legitimate.status, legitimate.header.Get("Location") != "")
	}
	location, err := url.Parse(legitimate.header.Get("Location"))
	if err != nil || location.Host != "chatgpt.com" || location.Path != "/connector_platform_oauth_redirect" || location.Query().Get("state") != "session-isolation-a-20260922" || location.Query().Get("iss") != quotaHTTPIssuer || location.Query().Get("code") == "" {
		t.Fatalf("callback legítimo inválido: parse_ok=%t host=%q path=%q state_ok=%t iss_ok=%t code_present=%t", err == nil, location.Host, location.Path, location.Query().Get("state") == "session-isolation-a-20260922", location.Query().Get("iss") == quotaHTTPIssuer, location.Query().Get("code") != "")
	}
	server.mu.Lock()
	codeCount := len(server.codes)
	_, issuedA := server.issued[clientA]
	completedA, completedOK := server.terminal[consentA.id]
	_, bStillPending := server.pending[consentB.id]
	server.mu.Unlock()
	if codeCount != 1 || issuedA || !completedOK || completedA.snapshot.Status != "COMPLETED" || !bStillPending {
		t.Fatalf("conclusão legítima alterou estado inesperado: codes=%d issued=%t completed=%t status=%q b_pending=%t", codeCount, issuedA, completedOK, completedA.snapshot.Status, bStillPending)
	}
	remainingB, remainingBSnapshot := readGroup8HTTPStatus(t, client, consentB.id, consentB.cookie)
	assertGroup8PublicResponse(t, remainingB, "PENDING", initialB.expiresAt)
	if remainingBSnapshot.status != "PENDING" {
		t.Fatal("B não permaneceu PENDING após a conclusão legítima de A")
	}

	replay := completeGroup8HTTP(t, client, consentA.id, consentA.csrf, consentA.cookie)
	if replay.status != http.StatusForbidden || replay.header.Get("Location") != "" || strings.Contains(string(replay.body), "code") {
		t.Fatalf("replay criou nova conclusão: http=%d location=%t", replay.status, replay.header.Get("Location") != "")
	}
	server.mu.Lock()
	finalCodeCount := len(server.codes)
	server.mu.Unlock()
	if finalCodeCount != 1 {
		t.Fatalf("replay alterou a quantidade de códigos: got=%d", finalCodeCount)
	}
	server.mu.Lock()
	issuedCount := len(server.issued)
	server.mu.Unlock()
	if issuedCount != 0 {
		t.Fatalf("conclusão sem troca por token marcou cliente como issued: count=%d", issuedCount)
	}
}
