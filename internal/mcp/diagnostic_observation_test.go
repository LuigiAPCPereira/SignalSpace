package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDiagnosticObservationRequiresAuthorizedValidTool(t *testing.T) {
	type event struct{ method, id string }
	events := make(chan event, 4)
	handler, err := NewOAuthHandler(OAuthConfig{
		ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: "owner-test",
		OnMCPEvent: func(method, id string) { events <- event{method, id} },
	}, fakeVerifier{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"connection_diagnostic","arguments":{}}}`
	response, result := oauthRequest(t, server, http.MethodPost, "/mcp", "", call, nil)
	if response.StatusCode != http.StatusOK || result["result"].(map[string]any)["isError"] != true {
		t.Fatal("unauthenticated tool did not return an auth challenge")
	}
	response, _ = oauthRequest(t, server, http.MethodPost, "/mcp", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing bearer accepted: %d", response.StatusCode)
	}
	response, _ = oauthRequest(t, server, http.MethodPost, "/mcp", "bearer-for-test", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"connection_diagnostic","arguments":{"path":"/"}}}`, nil)
	if response.StatusCode != http.StatusOK || len(events) != 0 {
		t.Fatal("invalid tool call produced an event")
	}

	response, _ = oauthRequest(t, server, http.MethodPost, "/mcp", "bearer-for-test", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatal("tool discovery failed")
	}
	select {
	case observed := <-events:
		if observed.method != "tools/list" || observed.id != "" {
			t.Fatalf("unexpected discovery event: %+v", observed)
		}
	case <-time.After(time.Second):
		t.Fatal("missing discovery event")
	}

	var previousID string
	for i := 0; i < 2; i++ {
		response, body := oauthRequest(t, server, http.MethodPost, "/mcp", "bearer-for-test", call, nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("diagnostic status=%d", response.StatusCode)
		}
		result := body["result"].(map[string]any)
		var diagnostic map[string]any
		if err := json.Unmarshal([]byte(result["content"].([]any)[0].(map[string]any)["text"].(string)), &diagnostic); err != nil {
			t.Fatal(err)
		}
		id, ok := diagnostic["diagnosticID"].(string)
		if !ok || len(id) != 32 || id == previousID {
			t.Fatalf("invalid diagnostic correlation: %v", diagnostic)
		}
		if _, claimed := diagnostic["chatgptVerified"]; claimed {
			t.Fatal("server cannot attest caller is ChatGPT")
		}
		select {
		case observed := <-events:
			if observed.method != "tools/call" || observed.id != id {
				t.Fatalf("response/log correlation mismatch: %+v versus %s", observed, id)
			}
		case <-time.After(time.Second):
			t.Fatal("missing tool call event")
		}
		previousID = id
	}
	if len(events) != 0 {
		t.Fatal("unexpected extra events")
	}
}
