package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

const (
	ssBE007AdminBase  = "/api/admin/v1"
	ssBE007PublicHost = "signalspace.example"
	ssBE007AdminHost  = "localhost:7677"
	ssBE007Fixture    = "SIGNALSPACE_READ_FIXTURE_SENTINEL"
)

type ssBE007HTTPResponse struct {
	status  int
	body    []byte
	head    http.Header
	cookies []*http.Cookie
}

func ssBE007Request(t *testing.T, client *http.Client, base, host, method, path, body, contentType, origin, csrf string, cookie *http.Cookie, bearer string, mcpRequest bool) ssBE007HTTPResponse {
	t.Helper()
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal("create loopback request")
	}
	req.Host = host
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if mcpRequest {
		req.Header.Set("MCP-Protocol-Version", "2025-06-18")
		req.Header.Set("Accept", "application/json, text/event-stream")
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal("loopback HTTP request failed")
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal("read loopback HTTP response")
	}
	return ssBE007HTTPResponse{status: response.StatusCode, body: payload, head: response.Header.Clone(), cookies: response.Cookies()}
}

func ssBE007Consent(t *testing.T, client *http.Client, query url.Values) (string, string, *http.Cookie) {
	t.Helper()
	response := ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodGet, "/authorize?"+query.Encode(), "", "", "", "", nil, "", false)
	csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindSubmatch(response.body)
	requestID := regexp.MustCompile(`approve ([A-Za-z0-9_-]{22})`).FindSubmatch(response.body)
	if response.status != http.StatusOK || len(csrf) != 2 || len(requestID) != 2 || len(response.cookies) != 1 {
		t.Fatalf("public OAuth consent did not produce its request and session cookie: status=%d", response.status)
	}
	return string(requestID[1]), string(csrf[1]), response.cookies[0]
}

func ssBE007MCP(t *testing.T, client *http.Client, method string, params any, token string, cookie *http.Cookie) ssBE007HTTPResponse {
	t.Helper()
	request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		request["params"] = params
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal("encode MCP request")
	}
	return ssBE007Request(t, client, "http://"+admin.PublicAddress, ssBE007PublicHost, http.MethodPost, "/mcp", string(body), "application/json", "", "", cookie, token, true)
}

func ssBE007ToolText(t *testing.T, response ssBE007HTTPResponse, wantError bool) string {
	t.Helper()
	if response.status != http.StatusOK {
		t.Fatalf("MCP tool call returned HTTP %d", response.status)
	}
	var payload struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if json.Unmarshal(response.body, &payload) != nil || len(payload.Result.Content) != 1 || payload.Result.IsError != wantError {
		t.Fatal("unexpected MCP tool result envelope")
	}
	return payload.Result.Content[0].Text
}

func TestReadOAuthPanelRevocationAcrossActualListeners(t *testing.T) {
	handler, authorization, console, err := embeddedHandlerWithWorkspace(readTestResource, filepath.Join(t.TempDir(), "identity"), compositionRead)
	if err != nil {
		t.Fatal("compose read mode")
	}
	gate, pairingCode, err := admin.NewGate()
	if err != nil {
		_ = console.Close()
		_ = authorization.Close()
		t.Fatal("create disposable admin gate")
	}
	readPlan, err := planComposition(compositionRead)
	if err != nil {
		gate.Close()
		_ = console.Close()
		_ = authorization.Close()
		t.Fatal("resolve canonical read composition")
	}
	listeners, err := reserveQuickPortsForPlan(readPlan, true)
	if err != nil {
		gate.Close()
		_ = console.Close()
		_ = authorization.Close()
		if errors.Is(err, syscall.EADDRINUSE) {
			t.Skipf("required loopback ports are occupied; no process was stopped: %v", err)
		}
		t.Fatalf("reserve actual public/admin listeners: %v", err)
	}
	publicServer := diagnosticServer(handler)
	adminServer := admin.NewServer(gate.HandlerWithRequests(authorization))
	publicDone := make(chan error, 1)
	adminDone := make(chan error, 1)
	go func() { publicDone <- publicServer.Serve(listeners.Public) }()
	go func() { adminDone <- adminServer.Serve(listeners.Admin) }()
	transport := &http.Transport{Proxy: nil}
	client := &http.Client{
		Transport: transport, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	var stopOnce sync.Once
	var stopErr error
	stop := func() {
		stopOnce.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := adminServer.Shutdown(ctx); err != nil {
				stopErr = fmt.Errorf("admin shutdown: %w", err)
				_ = adminServer.Close()
			}
			if err := publicServer.Shutdown(ctx); err != nil {
				stopErr = errors.Join(stopErr, fmt.Errorf("public shutdown: %w", err))
				_ = publicServer.Close()
			}
			_ = listeners.Close()
			for _, result := range []struct {
				name string
				ch   <-chan error
			}{{"public", publicDone}, {"admin", adminDone}} {
				select {
				case serveErr := <-result.ch:
					if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && !errors.Is(serveErr, net.ErrClosed) {
						stopErr = errors.Join(stopErr, fmt.Errorf("%s serve: %w", result.name, serveErr))
					}
				case <-ctx.Done():
					stopErr = errors.Join(stopErr, fmt.Errorf("%s server did not stop before deadline", result.name))
				}
			}
			transport.CloseIdleConnections()
			_ = console.Close()
			_ = authorization.Close()
			gate.Close()
		})
	}
	t.Cleanup(stop)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte(ssBE007Fixture+"\n"), 0600); err != nil {
		t.Fatal("write controlled workspace fixture")
	}
	publicBase := "http://" + admin.PublicAddress
	adminBase := "http://" + admin.AdminAddress

	// As rotas dos dois serviços não atravessam a fronteira de listener.
	publicAdmin := ssBE007Request(t, client, publicBase, ssBE007PublicHost, http.MethodGet, ssBE007AdminBase+"/session", "", "", "", "", nil, "", false)
	if publicAdmin.status != http.StatusNotFound || len(publicAdmin.cookies) != 0 {
		t.Fatalf("public listener exposed admin session route: status=%d", publicAdmin.status)
	}
	adminMCP := ssBE007Request(t, client, adminBase, ssBE007AdminHost, http.MethodPost, "/mcp", `{}`, "application/json", admin.AdminOrigin, "", nil, "", true)
	if adminMCP.status != http.StatusNotFound {
		t.Fatalf("admin listener exposed MCP route: status=%d", adminMCP.status)
	}

	registration := fmt.Sprintf(`{"client_name":"SS-BE-007 disposable read client","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	registeredResponse := ssBE007Request(t, client, publicBase, ssBE007PublicHost, http.MethodPost, "/register", registration, "application/json", "", "", nil, "", false)
	var registered struct {
		ClientID string `json:"client_id"`
	}
	if registeredResponse.status != http.StatusCreated || json.Unmarshal(registeredResponse.body, &registered) != nil || registered.ClientID == "" {
		t.Fatalf("DCR over public listener failed: status=%d", registeredResponse.status)
	}

	challenge := sha256.Sum256([]byte(readTestVerifier))
	diagnosticState := "ss-be-007-diagnostic-state"
	diagnosticQuery := url.Values{
		"client_id": {registered.ClientID}, "redirect_uri": {readTestCallback},
		"response_type": {"code"}, "scope": {"signalspace:diagnostic"},
		"resource": {readTestResource}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"}, "state": {diagnosticState},
	}
	diagnosticID, diagnosticCSRF, diagnosticCookie := ssBE007Consent(t, client, diagnosticQuery)
	if err := authorization.DecideTerminal(diagnosticID, true); err != nil {
		t.Fatal("complete diagnostic eligibility approval")
	}
	diagnosticForm := url.Values{"request": {diagnosticID}, "csrf": {diagnosticCSRF}}
	diagnosticComplete := ssBE007Request(t, client, publicBase, ssBE007PublicHost, http.MethodPost, "/authorize/complete", diagnosticForm.Encode(), "application/x-www-form-urlencoded", "", "", diagnosticCookie, "", false)
	diagnosticRedirect, err := url.Parse(diagnosticComplete.head.Get("Location"))
	if diagnosticComplete.status != http.StatusSeeOther || err != nil || diagnosticRedirect.Query().Get("code") == "" || diagnosticRedirect.Query().Get("state") != diagnosticState {
		t.Fatal("diagnostic OAuth completion failed")
	}
	diagnosticExchange := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {registered.ClientID},
		"redirect_uri": {readTestCallback}, "code": {diagnosticRedirect.Query().Get("code")},
		"code_verifier": {readTestVerifier}, "resource": {readTestResource},
	}
	diagnosticTokenResponse := ssBE007Request(t, client, publicBase, ssBE007PublicHost, http.MethodPost, "/token", diagnosticExchange.Encode(), "application/x-www-form-urlencoded", "", "", nil, "", false)
	var diagnosticToken struct {
		Token string `json:"access_token"`
		Scope string `json:"scope"`
	}
	if diagnosticTokenResponse.status != http.StatusOK || json.Unmarshal(diagnosticTokenResponse.body, &diagnosticToken) != nil || diagnosticToken.Token == "" || diagnosticToken.Scope != "signalspace:diagnostic" || len(authorization.IssuedClients()) != 1 {
		t.Fatal("diagnostic OAuth did not make exactly one client eligible")
	}

	var consoleOutput strings.Builder
	console.handleWorkspaceCommand("workspace request "+registered.ClientID+" "+root, &consoleOutput)
	if console.pending == nil {
		t.Fatal("local terminal workspace request was not created")
	}
	console.handleWorkspaceCommand("workspace approve "+console.pending.id, &consoleOutput)
	sessionMatch := regexp.MustCompile(`session=([a-f0-9]{32})`).FindStringSubmatch(consoleOutput.String())
	grant, err := console.grants.Snapshot()
	if err != nil || !grant.Active || grant.ClientID != registered.ClientID || !reflect.DeepEqual(grant.Scopes, []string{workspace.ScopeRead}) || len(sessionMatch) != 2 || grant.SessionID != sessionMatch[1] {
		t.Fatal("terminal approval did not create exactly one read-only workspace grant")
	}
	var statusOutput strings.Builder
	console.handleWorkspaceCommand("workspace status", &statusOutput)
	if !strings.Contains(statusOutput.String(), "Escopos exatos: "+workspace.ScopeRead) || strings.Contains(statusOutput.String(), workspace.ScopeWrite) || strings.Contains(statusOutput.String(), root) {
		t.Fatal("terminal workspace status exposed an incorrect scope or local root")
	}

	readScope := "signalspace:diagnostic " + workspace.ScopeRead
	readState := "ss-be-007-read-state"
	readQuery := url.Values{
		"client_id": {registered.ClientID}, "redirect_uri": {readTestCallback},
		"response_type": {"code"}, "scope": {readScope}, "resource": {readTestResource},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"}, "state": {readState},
	}
	readID, readCSRF, readCookie := ssBE007Consent(t, client, readQuery)

	bootstrap := ssBE007Request(t, client, adminBase, ssBE007AdminHost, http.MethodGet, ssBE007AdminBase+"/session", "", "", "", "", nil, "", false)
	var bootstrapSession struct {
		CSRF string `json:"csrf_token"`
	}
	if bootstrap.status != http.StatusOK || json.Unmarshal(bootstrap.body, &bootstrapSession) != nil || bootstrapSession.CSRF == "" {
		t.Fatalf("admin bootstrap over actual listener failed: status=%d", bootstrap.status)
	}
	bootstrapCookie := adminHTTPCookie(t, bootstrap.cookies, "signalspace_admin_bootstrap")
	pairBody := fmt.Sprintf(`{"pairing_code":%q,"passphrase":"long-local-owner-passphrase"}`, pairingCode)
	paired := ssBE007Request(t, client, adminBase, ssBE007AdminHost, http.MethodPost, ssBE007AdminBase+"/pair", pairBody, "application/json", admin.AdminOrigin, bootstrapSession.CSRF, bootstrapCookie, "", false)
	var ownerSession struct {
		CSRF string `json:"csrf_token"`
	}
	if paired.status != http.StatusCreated || json.Unmarshal(paired.body, &ownerSession) != nil || ownerSession.CSRF == "" {
		t.Fatalf("admin pairing over actual listener failed: status=%d", paired.status)
	}
	ownerCookie := adminHTTPCookie(t, paired.cookies, "signalspace_admin_session")

	// Cookies do not cross into MCP; OAuth bearers do not unlock the panel.
	publicWithAdminCookie := ssBE007Request(t, client, publicBase, ssBE007PublicHost, http.MethodGet, ssBE007AdminBase+"/session", "", "", "", "", ownerCookie, "", false)
	adminWithBearer := ssBE007Request(t, client, adminBase, ssBE007AdminHost, http.MethodGet, ssBE007AdminBase+"/requests", "", "", "", "", nil, diagnosticToken.Token, false)
	mcpWithAdminCookie := ssBE007MCP(t, client, "tools/list", nil, "", ownerCookie)
	if publicWithAdminCookie.status != http.StatusNotFound || len(publicWithAdminCookie.cookies) != 0 || adminWithBearer.status != http.StatusUnauthorized || mcpWithAdminCookie.status != http.StatusUnauthorized {
		t.Fatalf("cross-listener credentials crossed boundaries: public=%d admin=%d mcp=%d", publicWithAdminCookie.status, adminWithBearer.status, mcpWithAdminCookie.status)
	}

	adminQueue := ssBE007Request(t, client, adminBase, ssBE007AdminHost, http.MethodGet, ssBE007AdminBase+"/requests", "", "", "", "", ownerCookie, "", false)
	var queued struct {
		Requests []auth.RequestSnapshot `json:"requests"`
	}
	if adminQueue.status != http.StatusOK || json.Unmarshal(adminQueue.body, &queued) != nil || len(queued.Requests) != 2 || strings.Contains(string(adminQueue.body), root) {
		t.Fatal("admin queue did not return only OAuth metadata without the workspace root")
	}
	var queuedRead auth.RequestSnapshot
	for _, item := range queued.Requests {
		if item.ID == readID {
			queuedRead = item
			break
		}
	}
	if queuedRead.ID != readID || queuedRead.Version != 1 || queuedRead.Status != "PENDING" || queuedRead.Client.ID != registered.ClientID || queuedRead.Scope != readScope || !queuedRead.WorkspaceRead.Required || queuedRead.WorkspaceRead.GrantStatus != "ACTIVE" {
		t.Fatal("admin queue omitted the pending exact client/scope/version/active grant")
	}

	adminDetailPath := ssBE007AdminBase + "/requests/" + readID
	adminDetail := ssBE007Request(t, client, adminBase, ssBE007AdminHost, http.MethodGet, adminDetailPath, "", "", "", "", ownerCookie, "", false)
	var requestSnapshot auth.RequestSnapshot
	if adminDetail.status != http.StatusOK || json.Unmarshal(adminDetail.body, &requestSnapshot) != nil || requestSnapshot.ID != readID || requestSnapshot.Version != 1 || requestSnapshot.Status != "PENDING" || requestSnapshot.Client.ID != registered.ClientID || requestSnapshot.Scope != readScope || !requestSnapshot.WorkspaceRead.Required || requestSnapshot.WorkspaceRead.GrantStatus != "ACTIVE" || strings.Contains(string(adminDetail.body), root) {
		t.Fatal("admin request snapshot did not report the active read grant without exposing its root")
	}
	decision := fmt.Sprintf(`{"decision":"approve","expected_version":%d}`, requestSnapshot.Version)
	decisionResponse := ssBE007Request(t, client, adminBase, ssBE007AdminHost, http.MethodPost, adminDetailPath+"/decision", decision, "application/json", admin.AdminOrigin, ownerSession.CSRF, ownerCookie, "", false)
	var approved auth.RequestSnapshot
	var decisionFields map[string]json.RawMessage
	if decisionResponse.status != http.StatusOK || json.Unmarshal(decisionResponse.body, &approved) != nil || json.Unmarshal(decisionResponse.body, &decisionFields) != nil || approved.Status != "APPROVED" || approved.Version != 2 || approved.WorkspaceRead.GrantStatus != "ACTIVE" || decisionFields["code"] != nil || decisionFields["access_token"] != nil || len(authorization.IssuedClients()) != 1 {
		t.Fatal("admin approval emitted OAuth credentials or failed to record only the decision")
	}

	// A valid OAuth token is minted only by public completion and one-time exchange.
	completedForm := url.Values{"request": {readID}, "csrf": {readCSRF}}
	completed := ssBE007Request(t, client, publicBase, ssBE007PublicHost, http.MethodPost, "/authorize/complete", completedForm.Encode(), "application/x-www-form-urlencoded", "", "", readCookie, "", false)
	readRedirect, err := url.Parse(completed.head.Get("Location"))
	if completed.status != http.StatusSeeOther || err != nil || readRedirect.Host != "chatgpt.com" || readRedirect.Query().Get("state") != readState || readRedirect.Query().Get("code") == "" {
		t.Fatal("public OAuth completion did not issue the read callback code")
	}
	readExchange := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {registered.ClientID},
		"redirect_uri": {readTestCallback}, "code": {readRedirect.Query().Get("code")},
		"code_verifier": {readTestVerifier}, "resource": {readTestResource},
	}
	tokenResponse := ssBE007Request(t, client, publicBase, ssBE007PublicHost, http.MethodPost, "/token", readExchange.Encode(), "application/x-www-form-urlencoded", "", "", nil, "", false)
	var readToken struct {
		Token string `json:"access_token"`
		Scope string `json:"scope"`
	}
	if tokenResponse.status != http.StatusOK || json.Unmarshal(tokenResponse.body, &readToken) != nil || readToken.Token == "" || readToken.Scope != readScope {
		t.Fatal("public OAuth exchange failed or minted a token with unexpected scope")
	}
	verifier, err := mcp.NewStaticJWTVerifier(authorization.PublicKey(), authorization.KeyID())
	if err != nil {
		t.Fatal("create local JWT verifier")
	}
	issuer := strings.TrimSuffix(readTestResource, "/mcp")
	if _, err := verifier.VerifyIdentity(context.Background(), readToken.Token, issuer, readTestResource, workspace.ScopeRead, authorization.OwnerSubject()); err != nil {
		t.Fatal("read token identity/scope verification failed")
	}

	toolsResponse := ssBE007MCP(t, client, "tools/list", nil, readToken.Token, nil)
	var toolsEnvelope struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if toolsResponse.status != http.StatusOK || json.Unmarshal(toolsResponse.body, &toolsEnvelope) != nil {
		t.Fatal("tools/list failed over the public listener")
	}
	var toolNames []string
	for _, tool := range toolsEnvelope.Result.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	if !reflect.DeepEqual(toolNames, []string{"connection_diagnostic", "read_file", "list_directory"}) {
		t.Fatalf("unexpected read-mode MCP tools: %v", toolNames)
	}

	readParams := map[string]any{"name": "read_file", "arguments": map[string]string{"session_id": grant.SessionID, "path": "readme.txt"}}
	if got := ssBE007ToolText(t, ssBE007MCP(t, client, "tools/call", readParams, readToken.Token, nil), false); got != ssBE007Fixture+"\n" {
		t.Fatal("read_file did not return the controlled fixture")
	}
	listParams := map[string]any{"name": "list_directory", "arguments": map[string]string{"session_id": grant.SessionID, "path": "."}}
	listing := ssBE007ToolText(t, ssBE007MCP(t, client, "tools/call", listParams, readToken.Token, nil), false)
	var listed struct {
		Entries []string `json:"entries"`
	}
	if json.Unmarshal([]byte(listing), &listed) != nil || !reflect.DeepEqual(listed.Entries, []string{"readme.txt"}) {
		t.Fatal("list_directory did not return only the controlled fixture name")
	}

	console.handleWorkspaceCommand("workspace revoke current", &consoleOutput)
	var revokedStatus strings.Builder
	console.handleWorkspaceCommand("workspace status", &revokedStatus)
	revokedGrant, err := console.grants.Snapshot()
	if err != nil || revokedGrant.Active || !strings.Contains(consoleOutput.String(), "workspace grant revoked") || !strings.Contains(revokedStatus.String(), "Concessão local: ausente.") {
		t.Fatal("terminal revoke current did not remove the local grant")
	}
	if _, err := verifier.VerifyIdentity(context.Background(), readToken.Token, issuer, readTestResource, workspace.ScopeRead, authorization.OwnerSubject()); err != nil {
		t.Fatal("revocation unexpectedly invalidated the still-cryptographically-valid JWT")
	}
	deniedRead := ssBE007ToolText(t, ssBE007MCP(t, client, "tools/call", readParams, readToken.Token, nil), true)
	deniedList := ssBE007ToolText(t, ssBE007MCP(t, client, "tools/call", listParams, readToken.Token, nil), true)
	if deniedRead != "Workspace read unavailable or not authorized." || deniedList != "Workspace directory listing unavailable or not authorized." || strings.Contains(deniedRead, ssBE007Fixture) || strings.Contains(deniedList, ssBE007Fixture) {
		t.Fatal("revoked token exposed fixture content or returned a non-generic denial")
	}

	completedDetail := ssBE007Request(t, client, adminBase, ssBE007AdminHost, http.MethodGet, adminDetailPath, "", "", "", "", ownerCookie, "", false)
	var finalSnapshot auth.RequestSnapshot
	if completedDetail.status != http.StatusOK || json.Unmarshal(completedDetail.body, &finalSnapshot) != nil || finalSnapshot.Status != "COMPLETED" || finalSnapshot.WorkspaceRead.GrantStatus != "REVOKED" {
		t.Fatal("admin detail did not reconcile completed OAuth request with the revoked grant")
	}
	readQuery.Set("state", "ss-be-007-after-revoke")
	requestCountBeforeDeniedConsent := len(authorization.ListRequestSnapshots())
	deniedConsent := ssBE007Request(t, client, publicBase, ssBE007PublicHost, http.MethodGet, "/authorize?"+readQuery.Encode(), "", "", "", "", nil, "", false)
	remainingGrant, err := console.grants.Snapshot()
	if deniedConsent.status != http.StatusForbidden || err != nil || remainingGrant.Active || len(authorization.IssuedClients()) != 1 || len(authorization.ListRequestSnapshots()) != requestCountBeforeDeniedConsent {
		t.Fatalf("revoked workspace authorization was recreated: status=%d", deniedConsent.status)
	}

	stop()
	if stopErr != nil {
		t.Fatalf("loopback servers did not stop cleanly: %v", stopErr)
	}
	for _, address := range []string{admin.PublicAddress, admin.AdminAddress} {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			t.Fatalf("listener remained bound after graceful shutdown (%s): %v", address, err)
		}
		_ = listener.Close()
	}
}
