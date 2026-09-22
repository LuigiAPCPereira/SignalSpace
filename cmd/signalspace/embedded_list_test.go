package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func embeddedDirectoryCall(t *testing.T, handler http.Handler, bearer, session, path string) (int, string, bool) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name": "list_directory", "arguments": map[string]string{"session_id": session, "path": path},
		},
	})
	result := readRequest(t, handler, http.MethodPost, "/mcp", string(body), "application/json", bearer, nil)
	if result.Code != http.StatusOK {
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
		t.Fatalf("unexpected directory response: %s %v", result.Body.String(), err)
	}
	return result.Code, payload.Result.Content[0].Text, payload.Result.Error
}

func TestEmbeddedDirectoryListingRequiresLocalConsentAndRevokes(t *testing.T) {
	handler, authorization, console, err := embeddedHandlerWithWorkspace(readTestResource, filepath.Join(t.TempDir(), "identity"), compositionRead)
	if err != nil {
		t.Fatal(err)
	}
	defer authorization.Close()
	defer console.Close()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("PRIVATE_FILE_CONTENT"), 0600); err != nil {
		t.Fatal(err)
	}
	registration := fmt.Sprintf(`{"client_name":"Directory test client","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	registered := readRequest(t, handler, http.MethodPost, "/register", registration, "application/json", "", nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("client registration failed: %d", registered.Code)
	}
	var client struct {
		ID string `json:"client_id"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &client); err != nil || client.ID == "" {
		t.Fatal("missing client ID")
	}
	diagnostic := authorizeClient(t, handler, func(id string) error { return authorization.Approve(id, true) }, client.ID, "signalspace:diagnostic")
	if code, text, failed := embeddedDirectoryCall(t, handler, diagnostic, strings.Repeat("0", 32), "."); code != 200 || !failed || strings.Contains(text, "readme.txt") {
		t.Fatal("diagnostic token exposed listing")
	}
	var output bytes.Buffer
	console.handleWorkspaceCommand("workspace request "+client.ID+" "+root, &output)
	if console.pending == nil {
		t.Fatal("local workspace request was not created")
	}
	console.handleWorkspaceCommand("workspace approve "+console.pending.id, &output)
	session := regexp.MustCompile(`session=([a-f0-9]{32})`).FindStringSubmatch(output.String())
	if len(session) != 2 {
		t.Fatal("missing approved session")
	}
	scope := "signalspace:diagnostic signalspace:workspace.read"
	token := authorizeClient(t, handler, func(id string) error { return authorization.Approve(id, true) }, client.ID, scope)
	listing := readRequest(t, handler, http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "application/json", token, nil)
	if listing.Code != 200 {
		t.Fatalf("discovery failed: %d", listing.Code)
	}
	var tools struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(listing.Body.Bytes(), &tools); err != nil || len(tools.Result.Tools) != 3 || tools.Result.Tools[2].Name != "list_directory" {
		t.Fatalf("directory tool not published exclusively in read mode: %s", listing.Body.String())
	}
	code, text, failed := embeddedDirectoryCall(t, handler, token, session[1], ".")
	var names struct {
		Entries []string `json:"entries"`
	}
	if err := json.Unmarshal([]byte(text), &names); code != 200 || failed || err != nil || !reflect.DeepEqual(names.Entries, []string{"readme.txt"}) || strings.Contains(text, "PRIVATE_FILE_CONTENT") {
		t.Fatalf("authorized list failed: %d %t %s %v", code, failed, text, err)
	}
	console.handleWorkspaceCommand("workspace revoke "+session[1], &output)
	if code, text, failed := embeddedDirectoryCall(t, handler, token, session[1], "."); code != 200 || !failed || text != "Workspace directory listing unavailable or not authorized." {
		t.Fatalf("revoked OAuth token retained listing: %d %t %s", code, failed, text)
	}
}
