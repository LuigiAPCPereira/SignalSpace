package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
)

func group6Register(t *testing.T, client *http.Client, name string) string {
	t.Helper()
	registration := fmt.Sprintf(`{"client_name":%q,"redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, name, readTestCallback)
	response := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodPost, "/register", registration, "application/json", "", "", nil, "", false)
	if response.status != http.StatusCreated {
		t.Fatalf("registro OAuth %q: HTTP %d", name, response.status)
	}
	var registered struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(response.body, &registered); err != nil || registered.ClientID == "" {
		t.Fatalf("registro OAuth %q sem client_id: %v", name, err)
	}
	return registered.ClientID
}

func group6Detail(t *testing.T, client *http.Client, ownerCookie *http.Cookie, requestID string) auth.RequestSnapshot {
	t.Helper()
	response := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodGet, ssBE007AdminBase+"/requests/"+requestID, "", "", "", "", ownerCookie, "", false)
	if response.status != http.StatusOK {
		t.Fatalf("detalhe administrativo %s: HTTP %d", requestID, response.status)
	}
	var snapshot auth.RequestSnapshot
	if err := json.Unmarshal(response.body, &snapshot); err != nil {
		t.Fatalf("detalhe administrativo %s inválido: %v", requestID, err)
	}
	return snapshot
}

func group6AssertPendingSnapshot(t *testing.T, snapshot auth.RequestSnapshot, clientID string) {
	t.Helper()
	if snapshot.ID == "" || snapshot.Version != 1 || snapshot.Status != "PENDING" || snapshot.Client.ID != clientID || snapshot.Client.Verified || snapshot.RedirectURI != readTestCallback || snapshot.Scope != compositionDiagnosticScope || snapshot.WorkspaceRead.Required || snapshot.WorkspaceRead.GrantStatus != "NOT_APPLICABLE" || snapshot.DecidedAt != nil {
		t.Fatalf("snapshot OAuth não corresponde ao pedido PENDING registrado: %+v", snapshot)
	}
}

func group6SameSnapshot(a, b auth.RequestSnapshot) bool {
	if a.ID != b.ID || a.Version != b.Version || a.Status != b.Status || a.Client != b.Client || a.RedirectURI != b.RedirectURI || a.Scope != b.Scope || a.WorkspaceRead != b.WorkspaceRead || !a.CreatedAt.Equal(b.CreatedAt) || !a.ExpiresAt.Equal(b.ExpiresAt) {
		return false
	}
	if (a.DecidedAt == nil) != (b.DecidedAt == nil) {
		return false
	}
	return a.DecidedAt == nil || a.DecidedAt.Equal(*b.DecidedAt)
}

func TestOAuthRequestsRemainIndependentAcrossConcurrentHTTPAndTerminalDecisions(t *testing.T) {
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

	composition := startRestartHTTPComposition(t, stateDir)
	defer func() {
		if err := composition.stop(); err != nil {
			t.Errorf("shutdown da composição: %v", err)
		}
	}()

	clientA := group6Register(t, client, "SS-BE-007 grupo 6 cliente A")
	clientB := group6Register(t, client, "SS-BE-007 grupo 6 cliente B")
	if clientA == clientB {
		t.Fatal("DCR devolveu o mesmo client_id para A e B")
	}
	aID, _, _ := ssBE007Consent(t, client, restartHTTPQuery(clientA, "group6-request-a-state"))
	bID, _, _ := ssBE007Consent(t, client, restartHTTPQuery(clientB, "group6-request-b-state"))
	if aID == bID {
		t.Fatal("OAuth criou o mesmo request_id para A e B")
	}

	bootstrapCSRF, bootstrapCookie := restartHTTPBootstrap(t, client)
	ownerCSRF, ownerCookie := restartHTTPPair(t, client, composition.pairingCode, bootstrapCSRF, bootstrapCookie)
	queue := restartHTTPQueue(t, client, ownerCookie)
	if len(queue) != 2 {
		t.Fatalf("fila inicial não contém exatamente A e B: %d", len(queue))
	}
	byID := make(map[string]auth.RequestSnapshot, len(queue))
	for _, item := range queue {
		byID[item.ID] = item
	}
	if len(byID) != 2 {
		t.Fatalf("fila inicial contém IDs duplicados ou inesperados: %+v", queue)
	}
	group6AssertPendingSnapshot(t, byID[aID], clientA)
	group6AssertPendingSnapshot(t, byID[bID], clientB)
	aBefore := group6Detail(t, client, ownerCookie, aID)
	bBefore := group6Detail(t, client, ownerCookie, bID)
	group6AssertPendingSnapshot(t, aBefore, clientA)
	group6AssertPendingSnapshot(t, bBefore, clientB)

	forged := fmt.Sprintf(`{"decision":"approve","expected_version":1,"client_id":%q,"scope":%q,"redirect_uri":%q}`, clientB, bBefore.Scope, bBefore.RedirectURI)
	forgedResponse := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodPost, ssBE007AdminBase+"/requests/"+aID+"/decision", forged, "application/json", admin.AdminOrigin, ownerCSRF, ownerCookie, "", false)
	if forgedResponse.status != http.StatusBadRequest || !strings.Contains(string(forgedResponse.body), `"code":"INVALID_REQUEST"`) || strings.Contains(string(forgedResponse.body), clientB) || strings.Contains(string(forgedResponse.body), bBefore.RedirectURI) {
		t.Fatalf("payload com metadados forjados não foi rejeitado de forma estrita: HTTP %d body=%s", forgedResponse.status, forgedResponse.body)
	}
	aAfterForged := group6Detail(t, client, ownerCookie, aID)
	bAfterForged := group6Detail(t, client, ownerCookie, bID)
	if !group6SameSnapshot(aAfterForged, aBefore) || !group6SameSnapshot(bAfterForged, bBefore) {
		t.Fatalf("payload forjado contaminou snapshots: A=%+v B=%+v", aAfterForged, bAfterForged)
	}

	stale := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodPost, ssBE007AdminBase+"/requests/"+aID+"/decision", `{"decision":"approve","expected_version":2}`, "application/json", admin.AdminOrigin, ownerCSRF, ownerCookie, "", false)
	if stale.status != http.StatusConflict || !strings.Contains(string(stale.body), `"code":"STALE_REQUEST"`) {
		t.Fatalf("expected_version obsoleto não foi rejeitado: HTTP %d body=%s", stale.status, stale.body)
	}
	if got := group6Detail(t, client, ownerCookie, aID); !group6SameSnapshot(got, aBefore) {
		t.Fatalf("versão obsoleta alterou A: %+v", got)
	}
	if got := group6Detail(t, client, ownerCookie, bID); !group6SameSnapshot(got, bBefore) {
		t.Fatalf("versão obsoleta alterou B: %+v", got)
	}

	type terminalResult struct{ err error }
	start := make(chan struct{})
	terminalDone := make(chan terminalResult, 1)
	httpDone := make(chan ssBE007HTTPResponse, 1)
	go func() {
		<-start
		terminalDone <- terminalResult{err: composition.authorization.DecideTerminal(aID, false)}
	}()
	go func() {
		<-start
		httpDone <- ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodPost, ssBE007AdminBase+"/requests/"+aID+"/decision", `{"decision":"approve","expected_version":1}`, "application/json", admin.AdminOrigin, ownerCSRF, ownerCookie, "", false)
	}()
	close(start)
	terminalErr := (<-terminalDone).err
	httpResult := <-httpDone
	httpWon := httpResult.status == http.StatusOK && strings.Contains(string(httpResult.body), `"status":"APPROVED"`)
	terminalWon := terminalErr == nil
	if httpWon == terminalWon {
		t.Fatalf("corrida HTTP/terminal não teve exatamente um vencedor: HTTP=%d terminal=%v", httpResult.status, terminalErr)
	}
	if terminalWon && (httpResult.status != http.StatusConflict || !strings.Contains(string(httpResult.body), `"code":"ALREADY_DECIDED"`)) {
		t.Fatalf("terminal venceu, mas HTTP não recebeu rejeição da máquina de estados: HTTP %d body=%s", httpResult.status, httpResult.body)
	}
	if httpWon && !errors.Is(terminalErr, auth.ErrOAuthAlreadyDecided) {
		t.Fatalf("HTTP venceu, mas terminal recebeu erro inesperado: %v", terminalErr)
	}
	if !terminalWon && !errors.Is(terminalErr, auth.ErrOAuthAlreadyDecided) {
		t.Fatalf("terminal perdeu com erro inesperado: %v", terminalErr)
	}

	aAfterRace := group6Detail(t, client, ownerCookie, aID)
	if aAfterRace.Version != 2 || (aAfterRace.Status != "APPROVED" && aAfterRace.Status != "DENIED") || aAfterRace.DecidedAt == nil {
		t.Fatalf("estado final de A não representa uma decisão única: %+v", aAfterRace)
	}
	if httpWon && aAfterRace.Status != "APPROVED" {
		t.Fatalf("HTTP foi vencedor, mas A ficou %q", aAfterRace.Status)
	}
	if terminalWon && aAfterRace.Status != "DENIED" {
		t.Fatalf("terminal foi vencedor, mas A ficou %q", aAfterRace.Status)
	}
	if len(composition.authorization.IssuedClients()) != 0 {
		t.Fatal("decisão administrativa concorrente emitiu token antes da conclusão OAuth")
	}

	denyB := ssBE007Request(t, client, "http://"+admin.AdminAddress, ssBE007AdminHost, http.MethodPost, ssBE007AdminBase+"/requests/"+bID+"/decision", `{"decision":"deny","expected_version":1}`, "application/json", admin.AdminOrigin, ownerCSRF, ownerCookie, "", false)
	if denyB.status != http.StatusOK || !strings.Contains(string(denyB.body), `"status":"DENIED"`) || strings.Contains(string(denyB.body), "access_token") || strings.Contains(string(denyB.body), `"code"`) {
		t.Fatalf("decisão independente de B falhou ou expôs credencial: HTTP %d body=%s", denyB.status, denyB.body)
	}
	bAfterDecision := group6Detail(t, client, ownerCookie, bID)
	if bAfterDecision.Version != 2 || bAfterDecision.Status != "DENIED" || bAfterDecision.DecidedAt == nil || bAfterDecision.Client.ID != clientB || bAfterDecision.Scope != bBefore.Scope || bAfterDecision.RedirectURI != bBefore.RedirectURI {
		t.Fatalf("decisão de B não preservou seus metadados: %+v", bAfterDecision)
	}
	if got := group6Detail(t, client, ownerCookie, aID); !group6SameSnapshot(got, aAfterRace) {
		t.Fatalf("decisão de B contaminou A: antes=%+v depois=%+v", aAfterRace, got)
	}
	if len(composition.authorization.IssuedClients()) != 0 {
		t.Fatal("decisão de B emitiu token sem conclusão/troca OAuth")
	}
	if err := composition.stop(); err != nil {
		t.Fatalf("shutdown final da composição: %v", err)
	}
	assertRestartHTTPPortsFree(t)
}
