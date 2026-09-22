package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
)

const restartHTTPResource = "https://signalspace.example/mcp"

type restartHTTPComposition struct {
	authorization *auth.Server
	gate          *admin.Gate
	publicServer  *http.Server
	adminServer   *http.Server
	listeners     *admin.Listeners
	publicDone    chan error
	adminDone     chan error
	pairingCode   string
	stopOnce      sync.Once
	stopErr       error
}

func startRestartHTTPComposition(t *testing.T, stateDir string) *restartHTTPComposition {
	t.Helper()
	handler, authorization, console, err := embeddedHandlerWithWorkspace(restartHTTPResource, stateDir, compositionDiagnostic)
	if err != nil {
		t.Fatalf("composição DIAGNOSTIC: %v", err)
	}
	if console != nil {
		_ = authorization.Close()
		t.Fatal("composição DIAGNOSTIC criou console de workspace")
	}
	gate, pairingCode, err := admin.NewGate()
	if err != nil {
		_ = authorization.Close()
		t.Fatalf("Gate administrativo: %v", err)
	}
	plan, err := planComposition(compositionDiagnostic)
	if err != nil {
		gate.Close()
		_ = authorization.Close()
		t.Fatalf("plano DIAGNOSTIC: %v", err)
	}
	listeners, err := reserveQuickPortsForPlan(plan, true)
	if err != nil {
		gate.Close()
		_ = authorization.Close()
		if errors.Is(err, syscall.EADDRINUSE) {
			t.Skipf("BLOQUEADO: 7676/7677 ocupadas; nenhum processo de terceiro foi encerrado: %v", err)
		}
		t.Fatalf("reserva dos dois listeners: %v", err)
	}

	publicServer := diagnosticServer(handler)
	adminServer := admin.NewServer(gate.HandlerWithRequests(authorization))
	composition := &restartHTTPComposition{
		authorization: authorization,
		gate:          gate,
		publicServer:  publicServer,
		adminServer:   adminServer,
		listeners:     listeners,
		publicDone:    make(chan error, 1),
		adminDone:     make(chan error, 1),
		pairingCode:   pairingCode,
	}
	go func() { composition.publicDone <- publicServer.Serve(listeners.Public) }()
	go func() { composition.adminDone <- adminServer.Serve(listeners.Admin) }()
	return composition
}

func (c *restartHTTPComposition) stop() error {
	c.stopOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.adminServer.Shutdown(ctx); err != nil {
			c.stopErr = errors.Join(c.stopErr, fmt.Errorf("shutdown administrativo: %w", err))
			_ = c.adminServer.Close()
		}
		if err := c.publicServer.Shutdown(ctx); err != nil {
			c.stopErr = errors.Join(c.stopErr, fmt.Errorf("shutdown público: %w", err))
			_ = c.publicServer.Close()
		}
		_ = c.listeners.Close()
		for _, result := range []struct {
			name string
			ch   <-chan error
		}{{"público", c.publicDone}, {"administrativo", c.adminDone}} {
			select {
			case err := <-result.ch:
				if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
					c.stopErr = errors.Join(c.stopErr, fmt.Errorf("serve %s: %w", result.name, err))
				}
			case <-ctx.Done():
				c.stopErr = errors.Join(c.stopErr, fmt.Errorf("serve %s não encerrou no prazo", result.name))
			}
		}
		if err := c.authorization.Close(); err != nil {
			c.stopErr = errors.Join(c.stopErr, fmt.Errorf("fechamento OAuth: %w", err))
		}
		c.gate.Close()
	})
	return c.stopErr
}

func assertRestartHTTPPortsFree(t *testing.T) {
	t.Helper()
	for _, address := range []string{admin.PublicAddress, admin.AdminAddress} {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			t.Fatalf("listener residual em %s: %v", address, err)
		}
		_ = listener.Close()
	}
}

func restartHTTPIdentity(t *testing.T, client *http.Client) (string, string) {
	t.Helper()
	metadata := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodGet, "/.well-known/oauth-authorization-server", "", "", "", "", nil, "", false)
	if metadata.status != http.StatusOK {
		t.Fatalf("metadados OAuth: HTTP %d", metadata.status)
	}
	var metadataBody struct {
		Issuer string `json:"issuer"`
		JWKS   string `json:"jwks_uri"`
	}
	if err := json.Unmarshal(metadata.body, &metadataBody); err != nil || metadataBody.Issuer != strings.TrimSuffix(restartHTTPResource, "/mcp") || metadataBody.JWKS == "" {
		t.Fatalf("metadados OAuth inconsistentes: %v", err)
	}
	jwks := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodGet, "/oauth/jwks", "", "", "", "", nil, "", false)
	if jwks.status != http.StatusOK {
		t.Fatalf("JWKS OAuth: HTTP %d", jwks.status)
	}
	var jwksBody struct {
		Keys []struct {
			KeyID string `json:"kid"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(jwks.body, &jwksBody); err != nil || len(jwksBody.Keys) != 1 || jwksBody.Keys[0].KeyID == "" {
		t.Fatalf("identidade JWKS inválida: %v", err)
	}
	return metadataBody.Issuer, jwksBody.Keys[0].KeyID
}

func restartHTTPRegister(t *testing.T, client *http.Client) string {
	t.Helper()
	registration := fmt.Sprintf(`{"client_name":"SS-BE-007 restart client","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	response := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodPost, "/register", registration, "application/json", "", "", nil, "", false)
	if response.status != http.StatusCreated {
		t.Fatalf("DCR OAuth: HTTP %d", response.status)
	}
	var registered struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(response.body, &registered); err != nil || registered.ClientID == "" {
		t.Fatalf("DCR sem client_id: %v", err)
	}
	return registered.ClientID
}

func restartHTTPQuery(clientID, state string) url.Values {
	hash := sha256.Sum256([]byte(readTestVerifier))
	return url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {readTestCallback},
		"response_type":         {"code"},
		"scope":                 {"signalspace:diagnostic"},
		"resource":              {restartHTTPResource},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(hash[:])},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
}

func restartHTTPBootstrap(t *testing.T, client *http.Client) (string, *http.Cookie) {
	t.Helper()
	response := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodGet, ssBE007AdminBase+"/session", "", "", "", "", nil, "", false)
	if response.status != http.StatusOK {
		t.Fatalf("bootstrap administrativo: HTTP %d", response.status)
	}
	var model struct {
		State         string `json:"state"`
		Authenticated bool   `json:"authenticated"`
		CSRF          string `json:"csrf_token"`
	}
	if err := json.Unmarshal(response.body, &model); err != nil || model.State != "UNPAIRED" || model.Authenticated || model.CSRF == "" {
		t.Fatalf("painel não iniciou UNPAIRED: %v", err)
	}
	return model.CSRF, adminHTTPCookie(t, response.cookies, "signalspace_admin_bootstrap")
}

func restartHTTPPair(t *testing.T, client *http.Client, pairingCode, csrf string, bootstrapCookie *http.Cookie) (string, *http.Cookie) {
	t.Helper()
	body := fmt.Sprintf(`{"pairing_code":%q,"passphrase":"long-local-owner-passphrase"}`, pairingCode)
	response := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodPost, ssBE007AdminBase+"/pair", body, "application/json", admin.AdminOrigin, csrf, bootstrapCookie, "", false)
	if response.status != http.StatusCreated {
		t.Fatalf("pareamento administrativo: HTTP %d", response.status)
	}
	var model struct {
		State         string `json:"state"`
		Authenticated bool   `json:"authenticated"`
		CSRF          string `json:"csrf_token"`
	}
	if err := json.Unmarshal(response.body, &model); err != nil || model.State != "AUTHENTICATED" || !model.Authenticated || model.CSRF == "" {
		t.Fatalf("pareamento não autenticou: %v", err)
	}
	return model.CSRF, adminHTTPCookie(t, response.cookies, "signalspace_admin_session")
}

func restartHTTPQueue(t *testing.T, client *http.Client, cookie *http.Cookie) []auth.RequestSnapshot {
	t.Helper()
	response := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodGet, ssBE007AdminBase+"/requests", "", "", "", "", cookie, "", false)
	if response.status != http.StatusOK {
		t.Fatalf("fila administrativa: HTTP %d", response.status)
	}
	var queue struct {
		Requests []auth.RequestSnapshot `json:"requests"`
	}
	if err := json.Unmarshal(response.body, &queue); err != nil {
		t.Fatalf("fila administrativa inválida: %v", err)
	}
	return queue.Requests
}

func TestQuickPanelOAuthRestartDropsTransientAuthorizations(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "identity")
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	first := startRestartHTTPComposition(t, stateDir)
	defer func() {
		if err := first.stop(); err != nil {
			t.Errorf("shutdown da primeira composição: %v", err)
		}
	}()
	firstIssuer, firstKeyID := restartHTTPIdentity(t, client)
	clientID := restartHTTPRegister(t, client)

	aID, aCSRF, aCookie := ssBE007Consent(t, client, restartHTTPQuery(clientID, "restart-pending-a"))
	bID, bCSRF, bCookie := ssBE007Consent(t, client, restartHTTPQuery(clientID, "restart-code-b-state"))
	if aID == bID {
		t.Fatal("pedidos OAuth A e B não são distintos")
	}
	pending := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodGet, "/authorize/status?request_id="+url.QueryEscape(aID), "", "", "", "", aCookie, "", false)
	if pending.status != http.StatusOK || !strings.Contains(string(pending.body), `"status":"PENDING"`) {
		t.Fatalf("pedido A não ficou PENDING: HTTP %d", pending.status)
	}

	bootstrapCSRF, bootstrapCookie := restartHTTPBootstrap(t, client)
	ownerCSRF, ownerCookie := restartHTTPPair(t, client, first.pairingCode, bootstrapCSRF, bootstrapCookie)
	queue := restartHTTPQueue(t, client, ownerCookie)
	if len(queue) != 2 {
		t.Fatalf("primeira fila não contém exatamente A e B: %d", len(queue))
	}
	decisionPath := ssBE007AdminBase + "/requests/" + bID + "/decision"
	decision := `{"decision":"approve","expected_version":1}`
	approved := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodPost, decisionPath, decision, "application/json", admin.AdminOrigin, ownerCSRF, ownerCookie, "", false)
	var approvedSnapshot auth.RequestSnapshot
	if approved.status != http.StatusOK || json.Unmarshal(approved.body, &approvedSnapshot) != nil || approvedSnapshot.ID != bID || approvedSnapshot.Status != "APPROVED" || strings.Contains(string(approved.body), "access_token") || strings.Contains(string(approved.body), `"code"`) || len(first.authorization.IssuedClients()) != 0 {
		t.Fatalf("aprovação B alterou efeitos OAuth: HTTP %d", approved.status)
	}

	completed := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodPost, "/authorize/complete", url.Values{"request": {bID}, "csrf": {bCSRF}}.Encode(), "application/x-www-form-urlencoded", "", "", bCookie, "", false)
	redirect, err := url.Parse(completed.head.Get("Location"))
	if completed.status != http.StatusSeeOther || err != nil || redirect.Query().Get("code") == "" || redirect.Query().Get("state") != "restart-code-b-state" {
		t.Fatalf("conclusão OAuth B não emitiu redirect esperado: HTTP %d", completed.status)
	}
	bCode := redirect.Query().Get("code")
	completedDetail := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodGet, ssBE007AdminBase+"/requests/"+bID, "", "", "", "", ownerCookie, "", false)
	if completedDetail.status != http.StatusOK || !strings.Contains(string(completedDetail.body), `"status":"COMPLETED"`) {
		t.Fatalf("B não ficou COMPLETED na fonte administrativa: HTTP %d", completedDetail.status)
	}

	if err := first.stop(); err != nil {
		t.Fatal(err)
	}
	assertRestartHTTPPortsFree(t)

	second := startRestartHTTPComposition(t, stateDir)
	defer func() {
		if err := second.stop(); err != nil {
			t.Errorf("shutdown da segunda composição: %v", err)
		}
	}()
	secondIssuer, secondKeyID := restartHTTPIdentity(t, client)
	if secondIssuer != firstIssuer || secondKeyID != firstKeyID {
		t.Fatal("identidade OAuth pública não sobreviveu ao restart")
	}

	oldAdmin := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodGet, ssBE007AdminBase+"/session", "", "", "", "", ownerCookie, "", false)
	if oldAdmin.status != http.StatusUnauthorized || !strings.Contains(string(oldAdmin.body), `"AUTH_REQUIRED"`) || strings.Contains(string(oldAdmin.body), `"authenticated":true`) {
		t.Fatalf("cookie administrativo antigo autenticou após restart: HTTP %d", oldAdmin.status)
	}
	newBootstrapCSRF, newBootstrapCookie := restartHTTPBootstrap(t, client)
	newOwnerCSRF, newOwnerCookie := restartHTTPPair(t, client, second.pairingCode, newBootstrapCSRF, newBootstrapCookie)
	_ = newOwnerCSRF

	for _, old := range []struct {
		id     string
		cookie *http.Cookie
		csrf   string
	}{
		{aID, aCookie, aCSRF},
		{bID, bCookie, ""},
	} {
		status := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodGet, "/authorize/status?request_id="+url.QueryEscape(old.id), "", "", "", "", old.cookie, "", false)
		if status.status != http.StatusNotFound || strings.Contains(string(status.body), "access_token") || strings.Contains(string(status.body), `"code"`) {
			t.Fatalf("pedido OAuth antigo %s ainda foi exposto: HTTP %d", old.id, status.status)
		}
		if old.csrf != "" {
			late := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodPost, "/authorize/complete", url.Values{"request": {old.id}, "csrf": {old.csrf}}.Encode(), "application/x-www-form-urlencoded", "", "", old.cookie, "", false)
			if late.status != http.StatusForbidden || late.head.Get("Location") != "" || strings.Contains(string(late.body), "access_token") || strings.Contains(string(late.body), `"code"`) {
				t.Fatalf("conclusão tardia do pedido %s teve efeito: HTTP %d", old.id, late.status)
			}
		}
	}

	tokenExchange := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"redirect_uri":  {readTestCallback},
		"code":          {bCode},
		"code_verifier": {readTestVerifier},
		"resource":      {restartHTTPResource},
	}
	for attempt := 1; attempt <= 2; attempt++ {
		invalid := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodPost, "/token", tokenExchange.Encode(), "application/x-www-form-urlencoded", "", "", nil, "", false)
		if invalid.status != http.StatusBadRequest || !strings.Contains(string(invalid.body), `"error":"invalid_grant"`) || strings.Contains(string(invalid.body), "access_token") {
			t.Fatalf("troca do código B após restart na tentativa %d: HTTP %d", attempt, invalid.status)
		}
	}

	publicAdmin := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodGet, ssBE007AdminBase+"/session", "", "", "", "", newOwnerCookie, "", false)
	if publicAdmin.status != http.StatusNotFound || len(publicAdmin.cookies) != 0 {
		t.Fatalf("listener público expôs API administrativa após restart: HTTP %d", publicAdmin.status)
	}
	adminMCP := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodPost, "/mcp", `{}`, "application/json", admin.AdminOrigin, "", nil, "", true)
	if adminMCP.status != http.StatusNotFound {
		t.Fatalf("listener administrativo expôs MCP após restart: HTTP %d", adminMCP.status)
	}
	publicTools := ssBE007MCP(t, client, "tools/list", nil, "", nil)
	if publicTools.status != http.StatusUnauthorized || strings.Contains(string(publicTools.body), "read_file") || strings.Contains(string(publicTools.body), "replace_text") || strings.Contains(string(publicTools.body), "run_workspace_tests") {
		t.Fatalf("tools/list público não permaneceu fechado: HTTP %d", publicTools.status)
	}

	cID, cCSRF, cCookie := ssBE007Consent(t, client, restartHTTPQuery(clientID, "restart-new-c-state"))
	if cID == aID || cID == bID {
		t.Fatal("pedido C reutilizou identificador transitório anterior")
	}
	cStatus := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodGet, "/authorize/status?request_id="+url.QueryEscape(cID), "", "", "", "", cCookie, "", false)
	if cStatus.status != http.StatusOK || !strings.Contains(string(cStatus.body), `"status":"PENDING"`) {
		t.Fatalf("pedido C não exigiu consentimento novo: HTTP %d", cStatus.status)
	}
	queue = restartHTTPQueue(t, client, newOwnerCookie)
	if len(queue) != 1 || queue[0].ID != cID || queue[0].Status != "PENDING" || queue[0].Client.ID != clientID {
		t.Fatalf("fila após restart não contém somente C pendente: %+v", queue)
	}
	if strings.Contains(string(cStatus.body), aID) || strings.Contains(string(cStatus.body), bID) || cCSRF == "" {
		t.Fatal("estado de C expôs pedidos antigos ou não retornou CSRF próprio")
	}
	if len(second.authorization.IssuedClients()) != 0 {
		t.Fatal("restart restaurou cliente OAuth como cliente emitido")
	}
}
