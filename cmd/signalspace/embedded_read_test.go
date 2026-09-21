package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/tunnel"
)

const readTestResource = "https://signalspace.example/mcp"
const readTestCallback = "https://chatgpt.com/connector_platform_oauth_redirect"
const readTestVerifier = "valid-verifier-with-enough-length-to-meet-pkce-requirements-2026"

func readRequest(t *testing.T, handler http.Handler, method, path, body, contentType, bearer string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = "signalspace.example"
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if strings.HasPrefix(path, "/mcp") {
		request.Header.Set("MCP-Protocol-Version", "2025-06-18")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func authorizeClient(t *testing.T, handler http.Handler, approval func(string) error, clientID, scope string) string {
	t.Helper()
	hash := sha256.Sum256([]byte(readTestVerifier))
	query := url.Values{"client_id": {clientID}, "redirect_uri": {readTestCallback}, "response_type": {"code"}, "scope": {scope}, "resource": {readTestResource}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])}, "code_challenge_method": {"S256"}, "state": {"random-state-identifier-for-tests"}}
	consent := readRequest(t, handler, "GET", "/authorize?"+query.Encode(), "", "", "", nil)
	if consent.Code != 200 {
		t.Fatalf("authorize scope %q: %d %s", scope, consent.Code, consent.Body.String())
	}
	csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(consent.Body.String())
	requestID := regexp.MustCompile(`approve ([A-Za-z0-9_-]{22})`).FindStringSubmatch(consent.Body.String())
	cookies := consent.Result().Cookies()
	if len(csrf) != 2 || len(requestID) != 2 || len(cookies) != 1 {
		t.Fatal("missing authorization form")
	}
	if err := approval(requestID[1]); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"request": {requestID[1]}, "csrf": {csrf[1]}}
	complete := readRequest(t, handler, "POST", "/authorize/complete", form.Encode(), "application/x-www-form-urlencoded", "", cookies[0])
	if complete.Code != http.StatusSeeOther {
		t.Fatalf("complete: %d %s", complete.Code, complete.Body.String())
	}
	redirect, err := url.Parse(complete.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	exchange := url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "redirect_uri": {readTestCallback}, "code": {redirect.Query().Get("code")}, "code_verifier": {readTestVerifier}, "resource": {readTestResource}}
	result := readRequest(t, handler, "POST", "/token", exchange.Encode(), "application/x-www-form-urlencoded", "", nil)
	if result.Code != 200 {
		t.Fatalf("token: %d %s", result.Code, result.Body.String())
	}
	var payload struct {
		Token string `json:"access_token"`
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &payload); err != nil || payload.Token == "" || payload.Scope != scope {
		t.Fatalf("token scope: %+v %v", payload, err)
	}
	return payload.Token
}

func readToolCall(t *testing.T, handler http.Handler, bearer, session, path string) (int, string, bool) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "read_file", "arguments": map[string]string{"session_id": session, "path": path}}})
	result := readRequest(t, handler, "POST", "/mcp", string(raw), "application/json", bearer, nil)
	if result.Code != 200 {
		return result.Code, result.Body.String(), true
	}
	var payload struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Error bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &payload); err != nil || len(payload.Result.Content) != 1 {
		t.Fatalf("unexpected MCP response: %s %v", result.Body.String(), err)
	}
	return result.Code, payload.Result.Content[0].Text, payload.Result.Error
}

func TestEmbeddedCompositionSharesLocalGrantAcrossOAuthAndMCP(t *testing.T) {
	handler, authorization, console, err := embeddedHandlerWithWorkspace(readTestResource, filepath.Join(t.TempDir(), "identity"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer authorization.Close()
	defer console.Close()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("test project only"), 0600); err != nil {
		t.Fatal(err)
	}
	registration := fmt.Sprintf(`{"client_name":"Test client","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	registered := readRequest(t, handler, "POST", "/register", registration, "application/json", "", nil)
	if registered.Code != 201 {
		t.Fatalf("register: %d %s", registered.Code, registered.Body.String())
	}
	var client struct {
		ID string `json:"client_id"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &client); err != nil || client.ID == "" {
		t.Fatal("missing OAuth client")
	}
	// Nenhuma concessão pode existir só porque o cliente foi registrado.
	var output bytes.Buffer
	console.handleWorkspaceCommand("workspace request "+client.ID+" "+root, &output)
	if console.pending != nil {
		t.Fatal("unissued client requested workspace")
	}
	diagnostic := authorizeClient(t, handler, func(id string) error { return authorization.Approve(id, true) }, client.ID, "signalspace:diagnostic")
	if len(authorization.IssuedClients()) != 1 {
		t.Fatal("diagnostic client not issued")
	}
	if code, body, _ := readToolCall(t, handler, diagnostic, "unknown", "readme.txt"); code != 200 || strings.Contains(body, "test project") {
		t.Fatal("diagnostic token accessed file")
	}
	// O escopo adicional só pode ser solicitado depois da confirmação da pasta.
	scope := "signalspace:diagnostic signalspace:workspace.read"
	hash := sha256.Sum256([]byte(readTestVerifier))
	query := url.Values{"client_id": {client.ID}, "redirect_uri": {readTestCallback}, "response_type": {"code"}, "scope": {scope}, "resource": {readTestResource}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])}, "code_challenge_method": {"S256"}, "state": {"random-state-identifier-for-tests"}}
	if denied := readRequest(t, handler, "GET", "/authorize?"+query.Encode(), "", "", "", nil); denied.Code != 403 {
		t.Fatalf("read scope before grant: %d", denied.Code)
	}
	console.handleWorkspaceCommand("workspace request "+client.ID+" "+root, &output)
	if console.pending == nil {
		t.Fatal("missing local grant request")
	}
	console.handleWorkspaceCommand("workspace approve "+console.pending.id, &output)
	session := regexp.MustCompile(`session=([a-f0-9]{32})`).FindStringSubmatch(output.String())
	if len(session) != 2 || !console.grants.AllowsClient(client.ID) {
		t.Fatal("local grant not created")
	}
	token := authorizeClient(t, handler, func(id string) error { return authorization.Approve(id, true) }, client.ID, scope)
	// Uma segunda credencial OAuth real não pode reutilizar a sessão concedida.
	otherRegistration := readRequest(t, handler, "POST", "/register", registration, "application/json", "", nil)
	if otherRegistration.Code != 201 {
		t.Fatal("second OAuth client registration failed")
	}
	var otherClient struct {
		ID string `json:"client_id"`
	}
	if err := json.Unmarshal(otherRegistration.Body.Bytes(), &otherClient); err != nil {
		t.Fatal(err)
	}
	otherDiagnostic := authorizeClient(t, handler, func(id string) error { return authorization.Approve(id, true) }, otherClient.ID, "signalspace:diagnostic")
	if code, text, failed := readToolCall(t, handler, otherDiagnostic, session[1], "readme.txt"); code != 200 || !failed || strings.Contains(text, "test project") {
		t.Fatal("second client reused approved session")
	}
	if denied := readRequest(t, handler, "GET", "/authorize?"+func() string { q := query; q.Set("client_id", otherClient.ID); return q.Encode() }(), "", "", "", nil); denied.Code != 403 {
		t.Fatal("second client obtained read consent")
	}
	// O mesmo registro pode usar a concessão em chamadas de chats diferentes.
	for i := 0; i < 2; i++ {
		if code, text, failed := readToolCall(t, handler, token, session[1], "readme.txt"); code != 200 || failed || text != "test project only" {
			t.Fatalf("read rejected: %d %t %q", code, failed, text)
		}
	}
	console.handleWorkspaceCommand("workspace revoke "+session[1], &output)
	if console.grants.AllowsClient(client.ID) {
		t.Fatal("revoke did not clear OAuth permission")
	}
	if code, text, failed := readToolCall(t, handler, token, session[1], "readme.txt"); code != 200 || !failed || strings.Contains(text, "test project") {
		t.Fatal("revoked session readable with unexpired token")
	}
	if denied := readRequest(t, handler, "GET", "/authorize?"+query.Encode(), "", "", "", nil); denied.Code != 403 {
		t.Fatalf("new token after revoke: %d", denied.Code)
	}
}

func TestReadModeRequiresDistinctLocalPublication(t *testing.T) {
	for _, input := range []string{"PUBLICAR\n", "PUBLICAR LEITURAX\n", "NÃO\n"} {
		called := false
		var output bytes.Buffer
		err := runQuickWithMode(context.Background(), strings.NewReader(input), &output, func(context.Context) (*tunnel.Quick, error) {
			called = true
			return nil, fmt.Errorf("unexpected tunnel")
		}, nil, true)
		if err != nil || called || !strings.Contains(output.String(), "Conexão cancelada") {
			t.Fatalf("read mode published with input %q: %v", input, err)
		}
	}
}

func TestQuickReadModePublishesOnlyAfterExplicitConsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cloudflared")
	script := "#!/bin/sh\n[ \"$1\" = tunnel ] && [ \"$2\" = --url ] && [ \"$3\" = http://127.0.0.1:7676 ] || exit 2\necho 'https://test-host.trycloudflare.com' >&2\nexec sleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output synchronizedBuffer
	tested := make(chan error, 1)
	finished := make(chan error, 1)
	go func() {
		finished <- runQuickWithMode(ctx, strings.NewReader("PUBLICAR LEITURA\n"), &output, tunnel.Start, func(_ context.Context, resource string) (mcp.TransportReport, error) {
			if resource != "https://test-host.trycloudflare.com/mcp" {
				tested <- fmt.Errorf("unexpected resource")
				return mcp.TransportReport{}, fmt.Errorf("unexpected resource")
			}
			req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:7676/.well-known/oauth-protected-resource", nil)
			if err != nil {
				tested <- err
				return mcp.TransportReport{}, err
			}
			req.Host = "test-host.trycloudflare.com"
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				tested <- err
				return mcp.TransportReport{}, err
			}
			defer response.Body.Close()
			var metadata struct {
				Scopes []string `json:"scopes_supported"`
			}
			err = json.NewDecoder(response.Body).Decode(&metadata)
			if err == nil && (response.StatusCode != 200 || len(metadata.Scopes) != 2 || metadata.Scopes[1] != "signalspace:workspace.read") {
				err = fmt.Errorf("read scope not advertised in explicit mode")
			}
			tested <- err
			return mcp.TransportReport{ResourceURL: resource}, err
		}, true)
	}()
	select {
	case err := <-tested:
		if err != nil {
			t.Fatal(err)
		}
	case err := <-finished:
		t.Fatalf("quick read stopped before preflight: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("quick read did not reach preflight")
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("quick read failed to shut down")
	}
}
