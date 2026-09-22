package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTerminalOAuthApprovalCannotBypassWorkspaceRevoke(t *testing.T) {
	handler, authorization, console, err := embeddedHandlerWithWorkspace(readTestResource, filepath.Join(t.TempDir(), "identity"), compositionRead)
	if err != nil {
		t.Fatal(err)
	}
	defer authorization.Close()
	defer console.Close()

	registration := fmt.Sprintf(`{"client_name":"Test client","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	registered := readRequest(t, handler, http.MethodPost, "/register", registration, "application/json", "", nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("register: %d", registered.Code)
	}
	var client struct {
		ID string `json:"client_id"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &client); err != nil || client.ID == "" {
		t.Fatal("missing OAuth client ID")
	}
	// O cliente precisa primeiro concluir OAuth de diagnóstico para solicitar pasta.
	authorizeClient(t, handler, func(id string) error {
		var out bytes.Buffer
		serveTerminalCommands(authorization, nil, strings.NewReader("approve "+id+"\n"), &out)
		if !strings.Contains(out.String(), "authorization decision recorded") {
			return fmt.Errorf("diagnostic terminal decision failed: %s", out.String())
		}
		return nil
	}, client.ID, "signalspace:diagnostic")

	var out bytes.Buffer
	console.handleWorkspaceCommand("workspace request "+client.ID+" "+t.TempDir(), &out)
	if console.pending == nil {
		t.Fatal("workspace grant was not requested")
	}
	console.handleWorkspaceCommand("workspace approve "+console.pending.id, &out)
	session := regexp.MustCompile(`session=([a-f0-9]{32})`).FindStringSubmatch(out.String())
	if len(session) != 2 || !console.grants.AllowsClient(client.ID) {
		t.Fatal("workspace grant was not created")
	}

	hash := sha256.Sum256([]byte(readTestVerifier))
	query := url.Values{
		"client_id": {client.ID}, "redirect_uri": {readTestCallback},
		"response_type": {"code"}, "scope": {"signalspace:diagnostic signalspace:workspace.read"},
		"resource": {readTestResource}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])},
		"code_challenge_method": {"S256"}, "state": {"random-state-identifier-for-tests"},
	}
	consent := readRequest(t, handler, http.MethodGet, "/authorize?"+query.Encode(), "", "", "", nil)
	if consent.Code != http.StatusOK {
		t.Fatalf("read consent was not created: %d", consent.Code)
	}
	match := regexp.MustCompile(`approve ([A-Za-z0-9_-]{22})`).FindStringSubmatch(consent.Body.String())
	if len(match) != 2 || len(consent.Result().Cookies()) != 1 {
		t.Fatal("missing request-bound OAuth consent")
	}
	id := match[1]
	cookie := consent.Result().Cookies()[0]
	console.handleWorkspaceCommand("workspace revoke "+session[1], &out)
	if console.grants.AllowsClient(client.ID) {
		t.Fatal("workspace revoke did not take effect")
	}

	out.Reset()
	serveTerminalCommands(authorization, nil, strings.NewReader("approve "+id+"\n"), &out)
	if !strings.Contains(out.String(), "workspace grant required") || strings.Contains(out.String(), "authorization decision recorded") {
		t.Fatalf("terminal bypassed grant revocation: %q", out.String())
	}
	status := readRequest(t, handler, http.MethodGet, "/authorize/status?request_id="+id, "", "", "", cookie)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"PENDING"`) {
		t.Fatalf("failed approval mutated public state: %d %s", status.Code, status.Body.String())
	}
	out.Reset()
	serveTerminalCommands(authorization, nil, strings.NewReader("deny "+id+"\n"), &out)
	if !strings.Contains(out.String(), "authorization decision recorded") {
		t.Fatalf("terminal denial unavailable after revocation: %q", out.String())
	}
	status = readRequest(t, handler, http.MethodGet, "/authorize/status?request_id="+id, "", "", "", cookie)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"DENIED"`) {
		t.Fatalf("denial not reflected in public state: %d %s", status.Code, status.Body.String())
	}
}
