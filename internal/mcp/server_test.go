package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

const testToken = "local-diagnostic-token-for-tests-only-0123456789"

type testServer struct {
	url    string
	server *http.Server
	client *http.Client
}

func startTestServer(t *testing.T) testServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	handler, err := NewLocalHandler(testToken, port)
	if err != nil {
		t.Fatal(err)
	}
	s := &http.Server{Handler: handler}
	go func() { _ = s.Serve(listener) }()
	t.Cleanup(func() { _ = s.Close() })
	return testServer{url: "http://" + listener.Addr().String() + "/mcp", server: s, client: &http.Client{Timeout: time.Second * 3}}
}

func (s testServer) post(t *testing.T, body string, token string, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, s.url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		if key == "Host" {
			req.Host = value
		} else {
			req.Header.Set(key, value)
		}
	}
	res, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if len(data) != 0 && json.Unmarshal(data, &parsed) != nil {
		t.Fatalf("invalid JSON response: %s", data)
	}
	res.Body = io.NopCloser(bytes.NewReader(data))
	return res, parsed
}

func TestAuthentication(t *testing.T) {
	s := startTestServer(t)
	for _, token := range []string{"", "incorrect"} {
		res, _ := s.post(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, token, nil)
		if res.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(res.Header.Get("WWW-Authenticate"), "Bearer ") {
			t.Fatalf("unauthorized status=%d", res.StatusCode)
		}
	}
	if _, err := NewLocalHandler("short", 7676); err == nil {
		t.Fatal("accepted short credential")
	}
	if _, err := NewLocalHandler("", 7676); err == nil {
		t.Fatal("accepted missing credential")
	}
}

func TestDiagnostic(t *testing.T) {
	s := startTestServer(t)
	res, message := s.post(t, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`, testToken, nil)
	if res.StatusCode != 200 || message["id"] != float64(0) {
		t.Fatalf("initialize: %d %v", res.StatusCode, message)
	}
	result := message["result"].(map[string]any)
	if result["protocolVersion"] != "2025-06-18" {
		t.Fatalf("invalid version: %v", result)
	}
	_, message = s.post(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, testToken, nil)
	tools := message["result"].(map[string]any)["tools"].([]any)
	if tools[0].(map[string]any)["name"] != toolName {
		t.Fatal("diagnostic tool missing")
	}
	res, message = s.post(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"connection_diagnostic","arguments":{}}}`, testToken, nil)
	if res.StatusCode != 200 {
		t.Fatalf("call status=%d", res.StatusCode)
	}
	content := message["result"].(map[string]any)["content"].([]any)
	var diagnostic map[string]any
	if err := json.Unmarshal([]byte(content[0].(map[string]any)["text"].(string)), &diagnostic); err != nil {
		t.Fatal(err)
	}
	if diagnostic["connected"] != true || diagnostic["mode"] != "local_diagnostic" {
		t.Fatalf("incorrect diagnostic: %v", diagnostic)
	}
	if _, exists := diagnostic["chatgptVerified"]; exists {
		t.Fatalf("caller identity must not be claimed: %v", diagnostic)
	}
	if id, ok := diagnostic["diagnosticID"].(string); !ok || len(id) != 32 {
		t.Fatalf("missing correlation identifier: %v", diagnostic)
	}
}

func TestHostOriginAndInputBoundary(t *testing.T) {
	s := startTestServer(t)
	base := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	cases := []struct {
		body    string
		headers map[string]string
		status  int
	}{
		{base, map[string]string{"Host": "attacker.example"}, 403},
		{base, map[string]string{"Origin": "https://attacker.example"}, 403},
		{base, map[string]string{"MCP-Protocol-Version": ""}, 400},
		{`{broken`, nil, 400},
		{strings.Repeat("x", maxBodyBytes+1), nil, 413},
		{base + base, nil, 400},
	}
	for _, tc := range cases {
		res, _ := s.post(t, tc.body, testToken, tc.headers)
		if res.StatusCode != tc.status {
			t.Errorf("body prefix %.12q: got %d want %d", tc.body, res.StatusCode, tc.status)
		}
	}
}

func TestFailuresAndNotifications(t *testing.T) {
	s := startTestServer(t)
	_, message := s.post(t, `{"jsonrpc":"2.0","id":5,"method":"does-not-exist"}`, testToken, nil)
	if message["error"].(map[string]any)["code"] != float64(-32601) {
		t.Fatalf("unexpected error: %v", message)
	}
	_, message = s.post(t, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"connection_diagnostic","arguments":{"path":"/"}}}`, testToken, nil)
	if message["error"].(map[string]any)["code"] != float64(-32602) {
		t.Fatalf("unexpected arguments accepted: %v", message)
	}
	res, _ := s.post(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, testToken, nil)
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("notification status=%d", res.StatusCode)
	}
	data, _ := io.ReadAll(res.Body)
	if len(data) != 0 {
		t.Fatal("notification must not have a body")
	}
}
