//go:build signalspace_testtime

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
)

func TestAdminOAuthRealExpiryLifecycleWithTestTimeBridge(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "identity")
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.Transport.(*http.Transport).CloseIdleConnections()

	composition := startRestartHTTPComposition(t, stateDir)
	defer func() {
		if err := composition.stop(); err != nil {
			t.Errorf("shutdown da composição integrada: %v", err)
		}
		assertRestartHTTPPortsFree(t)
	}()

	unauthenticated := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodGet, ssBE007AdminBase+"/requests", "", "", "", "", nil, "", false)
	if unauthenticated.status != http.StatusUnauthorized || !strings.Contains(string(unauthenticated.body), `"AUTH_REQUIRED"`) || strings.Contains(string(unauthenticated.body), `"requests":[]`) {
		t.Fatalf("fila sem sessão não falhou fechado: HTTP %d body=%s", unauthenticated.status, unauthenticated.body)
	}

	clientID := restartHTTPRegister(t, client)
	requestID, requestCSRF, requestCookie := ssBE007Consent(t, client, restartHTTPQuery(clientID, "real-expiry-state"))
	bootstrapCSRF, bootstrapCookie := restartHTTPBootstrap(t, client)
	ownerCSRF, ownerCookie := restartHTTPPair(t, client, composition.pairingCode, bootstrapCSRF, bootstrapCookie)

	queue := restartHTTPQueue(t, client, ownerCookie)
	if len(queue) != 1 || queue[0].ID != requestID || queue[0].Status != "PENDING" {
		t.Fatalf("fila inicial não contém pedido PENDING real: %+v", queue)
	}
	detailPath := ssBE007AdminBase + "/requests/" + requestID
	pendingDetail := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodGet, detailPath, "", "", "", "", ownerCookie, "", false)
	var pendingSnapshot auth.RequestSnapshot
	if pendingDetail.status != http.StatusOK || json.Unmarshal(pendingDetail.body, &pendingSnapshot) != nil || pendingSnapshot.ID != requestID || pendingSnapshot.Status != "PENDING" {
		t.Fatalf("detalhe inicial não ficou PENDING: HTTP %d body=%s", pendingDetail.status, pendingDetail.body)
	}

	if err := composition.authorization.TestExpirePendingRequest(requestID); err != nil {
		t.Fatalf("vencer pedido PENDING pela ponte: %v", err)
	}

	expiredDetail := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodGet, detailPath, "", "", "", "", ownerCookie, "", false)
	var expiredSnapshot auth.RequestSnapshot
	if expiredDetail.status != http.StatusOK || json.Unmarshal(expiredDetail.body, &expiredSnapshot) != nil || expiredSnapshot.ID != requestID || expiredSnapshot.Status != "EXPIRED" {
		t.Fatalf("GET real não materializou EXPIRED: HTTP %d body=%s", expiredDetail.status, expiredDetail.body)
	}
	expiredQueue := restartHTTPQueue(t, client, ownerCookie)
	if len(expiredQueue) != 1 || expiredQueue[0].ID != requestID || expiredQueue[0].Status == "PENDING" {
		t.Fatalf("polling administrativo não reteve o tombstone EXPIRED: %+v", expiredQueue)
	}
	secondExpiredDetail := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodGet, detailPath, "", "", "", "", ownerCookie, "", false)
	var secondExpiredSnapshot auth.RequestSnapshot
	if secondExpiredDetail.status != http.StatusOK || json.Unmarshal(secondExpiredDetail.body, &secondExpiredSnapshot) != nil || secondExpiredSnapshot.Status != "EXPIRED" || !secondExpiredSnapshot.ExpiresAt.Equal(expiredSnapshot.ExpiresAt) {
		t.Fatalf("polling alterou o tombstone real: HTTP %d body=%s", secondExpiredDetail.status, secondExpiredDetail.body)
	}

	decisionPath := detailPath + "/decision"
	decision := `{"decision":"approve","expected_version":1}`
	expiredDecision := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodPost, decisionPath, decision, "application/json", admin.AdminOrigin, ownerCSRF, ownerCookie, "", false)
	if expiredDecision.status != http.StatusGone || !strings.Contains(string(expiredDecision.body), `"REQUEST_EXPIRED"`) {
		t.Fatalf("decisão do tombstone real não retornou 410 REQUEST_EXPIRED: HTTP %d body=%s", expiredDecision.status, expiredDecision.body)
	}
	if issued := composition.authorization.IssuedClients(); len(issued) != 0 || strings.Contains(string(expiredDecision.body), "access_token") {
		t.Fatalf("fluxo expirado criou efeito OAuth: issued_clients=%d body=%s", len(issued), expiredDecision.body)
	}

	if err := composition.authorization.TestExpireTerminalRetention(requestID); err != nil {
		t.Fatalf("vencer retenção do tombstone real pela ponte: %v", err)
	}
	removedDetail := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodGet, detailPath, "", "", "", "", ownerCookie, "", false)
	if removedDetail.status != http.StatusNotFound || !strings.Contains(string(removedDetail.body), `"REQUEST_NOT_FOUND"`) {
		t.Fatalf("limpeza real não removeu tombstone: HTTP %d body=%s", removedDetail.status, removedDetail.body)
	}
	removedDecision := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodPost, decisionPath, decision, "application/json", admin.AdminOrigin, ownerCSRF, ownerCookie, "", false)
	if removedDecision.status != http.StatusNotFound || !strings.Contains(string(removedDecision.body), `"REQUEST_NOT_FOUND"`) {
		t.Fatalf("decisão após limpeza não retornou 404 REQUEST_NOT_FOUND: HTTP %d body=%s", removedDecision.status, removedDecision.body)
	}
	removedQueue := restartHTTPQueue(t, client, ownerCookie)
	if len(removedQueue) != 0 {
		t.Fatalf("fila ainda expôs pedido removido: %+v", removedQueue)
	}

	if err := composition.authorization.TestExpirePendingRequest(requestID); !errors.Is(err, auth.ErrOAuthRequestNotFound) {
		t.Fatalf("ponte aceitou novamente ID removido: %v", err)
	}
	if requestCSRF == "" || requestCookie == nil {
		t.Fatal("/authorize não retornou CSRF e cookie reais")
	}
	publicStatus := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodGet, "/authorize/status?request_id="+url.QueryEscape(requestID), "", "", "", "", requestCookie, "", false)
	if publicStatus.status != http.StatusNotFound || strings.Contains(string(publicStatus.body), "access_token") {
		t.Fatalf("consulta pública pós-limpeza expôs efeito OAuth: HTTP %d body=%s", publicStatus.status, publicStatus.body)
	}
}
