package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/approval"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/policy"
)

const programmingAdminPassphrase = "long-local-owner-passphrase"

func TestProgrammingV2MCPHTTPApprovalRoundTripUsesAdminManager(t *testing.T) {
	plan, err := planComposition(compositionProgramming)
	if err != nil {
		t.Fatal(err)
	}
	var bridge mcp.ProgrammingAuthorizer
	handler, authorization, console, err := embeddedHandlerForPlanWithAuthorizer(readTestResource, filepath.Join(t.TempDir(), "identity"), plan, func() mcp.ProgrammingAuthorizer {
		return bridge
	})
	if err != nil {
		t.Fatal(err)
	}
	defer authorization.Close()
	defer console.Close()

	approvals := approval.New()
	defer approvals.Close()
	bridge = &programmingAuthorizer{owner: authorization.OwnerSubject(), grants: console.grants, policies: policy.NewEngine(), approvals: approvals}

	gate, pairingCode, err := admin.NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	adminServer := httptest.NewServer(admin.Handler(gate.HandlerWithRequestsAndCapabilityApprovalsAndPolicies(authorization, approvals, nil)))
	defer adminServer.Close()
	publicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Host = "signalspace.example"
		handler.ServeHTTP(w, r)
	}))
	defer publicServer.Close()

	registered := readRequest(t, handler, http.MethodPost, "/register", fmt.Sprintf(`{"client_name":"Programming HTTP approval test","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback), "application/json", "", nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", registered.Code, registered.Body.String())
	}
	var client struct {
		ID string `json:"client_id"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &client); err != nil || client.ID == "" {
		t.Fatalf("client registration: %v", err)
	}
	token := authorizeClient(t, handler, func(id string) error { return authorization.DecideTerminal(id, true) }, client.ID, compositionProgrammingScope)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sessionID, err := console.grants.GrantProgrammingCheckout(root, client.ID)
	if err != nil {
		t.Fatal(err)
	}

	listing := programmingPublicHTTP(t, publicServer.Client(), publicServer.URL, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var listed struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	if listing.status != http.StatusOK || json.Unmarshal(listing.body, &listed) != nil || len(listed.Result.Tools) != 20 || !strings.Contains(string(listing.body), `"signalspace:programming"`) {
		t.Fatalf("HTTP tools/list did not expose the exact Programming surface: %d %s", listing.status, listing.body)
	}

	call := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_file","arguments":{"session_id":%q,"path":"hello.txt"}}}`, sessionID)
	first := programmingPublicHTTP(t, publicServer.Client(), publicServer.URL, token, call)
	var firstEnvelope struct {
		Result struct {
			Structured map[string]any `json:"structuredContent"`
		} `json:"result"`
	}
	if first.status != http.StatusOK || json.Unmarshal(first.body, &firstEnvelope) != nil || firstEnvelope.Result.Structured["code"] != "LOCAL_APPROVAL_REQUIRED" {
		t.Fatalf("first MCP call did not create local approval: %d %s", first.status, first.body)
	}
	requestID, ok := firstEnvelope.Result.Structured["request_id"].(string)
	if !ok || requestID == "" {
		t.Fatalf("first MCP call omitted approval request ID: %s", first.body)
	}

	bootstrap := programmingAdminHTTP(t, adminServer.Client(), adminServer.URL, http.MethodGet, "/api/admin/v1/session", nil, nil, "")
	if bootstrap.status != http.StatusOK {
		t.Fatalf("admin bootstrap: %d %s", bootstrap.status, bootstrap.body)
	}
	var bootstrapModel struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(bootstrap.body, &bootstrapModel); err != nil || bootstrapModel.CSRF == "" || len(bootstrap.cookies) == 0 {
		t.Fatalf("admin bootstrap state: %v %s", err, bootstrap.body)
	}
	paired := programmingAdminHTTP(t, adminServer.Client(), adminServer.URL, http.MethodPost, "/api/admin/v1/pair", []byte(fmt.Sprintf(`{"pairing_code":%q,"passphrase":"%s"}`, pairingCode, programmingAdminPassphrase)), bootstrap.cookies[0], bootstrapModel.CSRF)
	if paired.status != http.StatusCreated || len(paired.cookies) == 0 {
		t.Fatalf("admin pair: %d %s", paired.status, paired.body)
	}
	var pairedModel struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(paired.body, &pairedModel); err != nil || pairedModel.CSRF == "" {
		t.Fatalf("admin pair state: %v %s", err, paired.body)
	}
	ownerCookie := programmingCookie(t, paired.cookies, "signalspace_admin_session")
	approvalsList := programmingAdminHTTP(t, adminServer.Client(), adminServer.URL, http.MethodGet, "/api/admin/v1/capability-approvals", nil, ownerCookie, "")
	if approvalsList.status != http.StatusOK || !strings.Contains(string(approvalsList.body), requestID) {
		t.Fatalf("admin approval list did not observe MCP request: %d %s", approvalsList.status, approvalsList.body)
	}
	decision := programmingAdminHTTP(t, adminServer.Client(), adminServer.URL, http.MethodPost, "/api/admin/v1/capability-approvals/"+requestID+"/decision", []byte(`{"expected_version":1,"decision":"ALLOW_ONCE"}`), ownerCookie, pairedModel.CSRF)
	if decision.status != http.StatusOK || !strings.Contains(string(decision.body), `"status":"APPROVED"`) || strings.Contains(string(decision.body), "permit") {
		t.Fatalf("admin approval decision: %d %s", decision.status, decision.body)
	}

	retry := programmingPublicHTTP(t, publicServer.Client(), publicServer.URL, token, call)
	if retry.status != http.StatusOK || !strings.Contains(string(retry.body), "hello") || strings.Contains(string(retry.body), "LOCAL_APPROVAL_REQUIRED") {
		t.Fatalf("exact MCP retry was not executed after HTTP approval: %d %s", retry.status, retry.body)
	}
	third := programmingPublicHTTP(t, publicServer.Client(), publicServer.URL, token, call)
	if third.status != http.StatusOK || !strings.Contains(string(third.body), "LOCAL_APPROVAL_REQUIRED") || strings.Contains(string(third.body), requestID) {
		t.Fatalf("one-shot permit was reusable or request was not renewed: %d %s", third.status, third.body)
	}
}

type programmingHTTPResponse struct {
	status  int
	body    []byte
	cookies []*http.Cookie
}

func programmingPublicHTTP(t *testing.T, client *http.Client, baseURL, bearer, body string) programmingHTTPResponse {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "signalspace.example"
	request.Header.Set("Authorization", "Bearer "+bearer)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("MCP-Protocol-Version", "2025-06-18")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return programmingHTTPResponse{status: response.StatusCode, body: bodyBytes, cookies: response.Cookies()}
}

func programmingAdminHTTP(t *testing.T, client *http.Client, baseURL, method, path string, body []byte, cookie *http.Cookie, csrf string) programmingHTTPResponse {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "localhost:7677"
	if method != http.MethodGet {
		request.Header.Set("Origin", admin.AdminOrigin)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", csrf)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return programmingHTTPResponse{status: response.StatusCode, body: bodyBytes, cookies: response.Cookies()}
}

func programmingCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.Value != "" && cookie.MaxAge != -1 {
			return cookie
		}
	}
	t.Fatalf("missing %s cookie", name)
	return nil
}
