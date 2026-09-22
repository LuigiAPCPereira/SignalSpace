package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicCompositionsKeepWorkspaceWriteUnpublished(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       compositionMode
		wantScopes []string
		wantTools  []string
	}{
		{"diagnostic", compositionDiagnostic, []string{"signalspace:diagnostic"}, []string{"connection_diagnostic"}},
		{"read", compositionRead, []string{"signalspace:diagnostic", "signalspace:workspace.read"}, []string{"connection_diagnostic", "read_file", "list_directory"}},
	} {
		name := tc.name
		t.Run(name, func(t *testing.T) {
			handler, authorization, console, err := embeddedHandlerWithWorkspace(readTestResource, filepath.Join(t.TempDir(), "identity"), tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if console != nil {
					_ = console.Close()
				}
				_ = authorization.Close()
			})

			metadataResponse := readRequest(t, handler, http.MethodGet, "/.well-known/oauth-authorization-server", "", "", "", nil)
			if metadataResponse.Code != http.StatusOK {
				t.Fatalf("metadata status=%d", metadataResponse.Code)
			}
			var metadata struct {
				Scopes []string `json:"scopes_supported"`
			}
			if err := json.Unmarshal(metadataResponse.Body.Bytes(), &metadata); err != nil {
				t.Fatal(err)
			}
			if len(metadata.Scopes) != len(tc.wantScopes) {
				t.Fatalf("public %s composition announced unexpected scopes: %v", name, metadata.Scopes)
			}
			for index, scope := range metadata.Scopes {
				if scope != tc.wantScopes[index] {
					t.Fatalf("public %s composition announced unexpected scopes: %v", name, metadata.Scopes)
				}
				if scope == "signalspace:workspace.write" {
					t.Fatalf("public %s composition announced workspace.write: %v", name, metadata.Scopes)
				}
			}

			registration := `{"client_name":"Promotion gate test","redirect_uris":["` + readTestCallback + `"],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`
			registered := readRequest(t, handler, http.MethodPost, "/register", registration, "application/json", "", nil)
			if registered.Code != http.StatusCreated {
				t.Fatalf("register: %d %s", registered.Code, registered.Body.String())
			}
			var client struct {
				ID string `json:"client_id"`
			}
			if err := json.Unmarshal(registered.Body.Bytes(), &client); err != nil || client.ID == "" {
				t.Fatalf("missing client ID: %v", err)
			}
			challenge := sha256.Sum256([]byte(readTestVerifier))
			writeRequest := url.Values{
				"client_id":             {client.ID},
				"redirect_uri":          {readTestCallback},
				"response_type":         {"code"},
				"scope":                 {"signalspace:diagnostic signalspace:workspace.write"},
				"resource":              {readTestResource},
				"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
				"code_challenge_method": {"S256"},
				"state":                 {"promotion-gate-write-request"},
			}
			if rejected := readRequest(t, handler, http.MethodGet, "/authorize?"+writeRequest.Encode(), "", "", "", nil); rejected.Code == http.StatusOK {
				t.Fatal("public composition accepted an OAuth workspace.write request")
			}
			token := authorizeClient(t, handler, func(id string) error {
				return authorization.DecideTerminal(id, true)
			}, client.ID, "signalspace:diagnostic")

			listing := readRequest(t, handler, http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "application/json", token, nil)
			if listing.Code != http.StatusOK {
				t.Fatalf("tools/list: %d %s", listing.Code, listing.Body.String())
			}
			var listed struct {
				Result struct {
					Tools []struct {
						Name string `json:"name"`
					}
				} `json:"result"`
			}
			if err := json.Unmarshal(listing.Body.Bytes(), &listed); err != nil {
				t.Fatal(err)
			}
			if len(listed.Result.Tools) != len(tc.wantTools) {
				t.Fatalf("public %s composition advertised unexpected tools: %s", name, listing.Body.String())
			}
			for index, tool := range listed.Result.Tools {
				if tool.Name != tc.wantTools[index] {
					t.Fatalf("public %s composition advertised unexpected tools: %s", name, listing.Body.String())
				}
			}

			writeCall := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"replace_text","arguments":{"session_id":"` + strings.Repeat("0", 32) + `","path":"file.txt","expected":"before","replacement":"after"}}}`
			writeResponse := readRequest(t, handler, http.MethodPost, "/mcp", writeCall, "application/json", token, nil)
			if writeResponse.Code != http.StatusOK || !strings.Contains(writeResponse.Body.String(), `"error"`) || strings.Contains(writeResponse.Body.String(), "Workspace text replaced") {
				t.Fatalf("public %s composition accepted replace_text: %d %s", name, writeResponse.Code, writeResponse.Body.String())
			}
		})
	}
}
