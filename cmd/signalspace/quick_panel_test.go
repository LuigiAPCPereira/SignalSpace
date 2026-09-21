package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/tunnel"
)

func TestQuickModeArgumentsRequireExplicitPanel(t *testing.T) {
	for _, tc := range []struct {
		args            []string
		read, panel, ok bool
	}{
		{[]string{"connect", "quick"}, false, false, true},
		{[]string{"connect", "quick", "read"}, true, false, true},
		{[]string{"connect", "quick", "panel"}, false, true, true},
		{[]string{"connect", "quick", "read", "panel"}, true, true, true},
		{[]string{"connect", "quick", "write"}, false, false, false},
		{[]string{"connect", "quick", "read", "write"}, false, false, false},
		{[]string{"connect", "quick", "panel", "read"}, false, false, false},
		{[]string{"connect", "quick", "read", "read"}, false, false, false},
		{[]string{"connect", "quick", "other"}, false, false, false},
		{[]string{"doctor", "transport"}, false, false, false},
	} {
		read, panel, ok := quickModeArgs(tc.args)
		if read != tc.read || panel != tc.panel || ok != tc.ok {
			t.Fatalf("arguments %q => read=%t panel=%t ok=%t", tc.args, read, panel, ok)
		}
	}
}

func TestQuickPanelRequiresSeparateConfirmationAndFailsBeforeTunnelOnBind(t *testing.T) {
	var output bytes.Buffer
	called := false
	starter := func(context.Context) (*tunnel.Quick, error) {
		called = true
		return nil, errors.New("tunnel must not start")
	}
	if err := runQuickWithOptions(context.Background(), strings.NewReader("PUBLICAR\n"), &output, starter, nil, false, true); err != nil || called || !strings.Contains(output.String(), "Conexão cancelada") {
		t.Fatalf("legacy confirmation enabled panel: err=%v called=%t", err, called)
	}
	occupied, err := net.Listen("tcp4", admin.AdminAddress)
	if err != nil {
		t.Skipf("administrative port unavailable for bind test: %v", err)
	}
	defer occupied.Close()
	output.Reset()
	if err := runQuickWithOptions(context.Background(), strings.NewReader("PUBLICAR PAINEL\n"), &output, starter, nil, false, true); err == nil || called || strings.Contains(output.String(), "Código de pareamento") || strings.Contains(output.String(), "Cole no ChatGPT Web") {
		t.Fatalf("admin bind failure allowed tunnel or leaked pairing: err=%v called=%t", err, called)
	}
	public, err := net.Listen("tcp4", admin.PublicAddress)
	if err != nil {
		t.Fatalf("partial public listener leaked after bind error: %v", err)
	}
	_ = public.Close()
}

func TestQuickPanelServesLocalAPIWithoutPublishingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "cloudflared")
	script := "#!/bin/sh\n[ \"$1\" = tunnel ] && [ \"$2\" = --url ] && [ \"$3\" = http://127.0.0.1:7676 ] || exit 2\necho 'https://panel-test.trycloudflare.com' >&2\nexec sleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output synchronizedBuffer
	verified := make(chan struct{})
	finished := make(chan error, 1)
	client := &http.Client{Timeout: 2 * time.Second}
	request := func(address, host, path string, cookie *http.Cookie) (*http.Response, string, error) {
		req, err := http.NewRequest(http.MethodGet, "http://"+address+path, nil)
		if err != nil {
			return nil, "", err
		}
		req.Host = host
		if cookie != nil {
			req.AddCookie(cookie)
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		return response, string(body), err
	}
	go func() {
		finished <- runQuickWithOptions(ctx, strings.NewReader("PUBLICAR PAINEL\n"), &output, tunnel.Start, func(_ context.Context, resource string) (mcp.TransportReport, error) {
			if resource != "https://panel-test.trycloudflare.com/mcp" {
				return mcp.TransportReport{}, fmt.Errorf("unexpected public resource: %s", resource)
			}
			public, body, err := request(admin.PublicAddress, "panel-test.trycloudflare.com", "/.well-known/oauth-protected-resource", nil)
			if err != nil || public.StatusCode != 200 || !strings.Contains(body, resource) {
				return mcp.TransportReport{}, fmt.Errorf("public OAuth failed: status=%v err=%v", public, err)
			}
			publicAdmin, _, err := request(admin.PublicAddress, "panel-test.trycloudflare.com", "/api/admin/v1/session", nil)
			if err != nil || publicAdmin.StatusCode != 404 || len(publicAdmin.Cookies()) != 0 {
				return mcp.TransportReport{}, fmt.Errorf("admin API exposed on public origin: err=%v", err)
			}
			private, body, err := request(admin.AdminAddress, "localhost:7677", "/api/admin/v1/session", nil)
			if err != nil || private.StatusCode != 200 || !strings.Contains(body, `"state":"UNPAIRED"`) {
				return mcp.TransportReport{}, fmt.Errorf("private API unavailable: err=%v", err)
			}
			privateMCP, _, err := request(admin.AdminAddress, "localhost:7677", "/mcp", nil)
			if err != nil || privateMCP.StatusCode != 404 {
				return mcp.TransportReport{}, fmt.Errorf("MCP exposed on admin origin: err=%v", err)
			}
			close(verified)
			return mcp.TransportReport{ResourceURL: resource}, nil
		}, false, true)
	}()
	select {
	case <-verified:
	case err := <-finished:
		t.Fatalf("Quick ended before both servers were verified: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("Quick panel never served both local origins")
	}
	deadline := time.After(3 * time.Second)
	for !strings.Contains(output.String(), "Cole no ChatGPT Web") {
		select {
		case err := <-finished:
			t.Fatalf("Quick stopped before announcing verified origin: %v", err)
		case <-deadline:
			t.Fatal("Quick did not report the validated public URL")
		case <-time.After(10 * time.Millisecond):
		}
	}
	match := regexp.MustCompile(`Código de pareamento desta instância \(somente neste terminal, expira em 5 minutos\): ([A-Za-z0-9_-]+)`).FindStringSubmatch(output.String())
	if len(match) != 2 || strings.Contains(output.String(), "https://panel-test.trycloudflare.com:7677") {
		t.Fatal("pairing code missing from terminal or private origin was advertised publicly")
	}
	sessionResponse, sessionBody, err := request(admin.AdminAddress, "localhost:7677", "/api/admin/v1/session", nil)
	var bootstrap struct {
		CSRF string `json:"csrf_token"`
	}
	if err != nil || sessionResponse.StatusCode != 200 || json.Unmarshal([]byte(sessionBody), &bootstrap) != nil || bootstrap.CSRF == "" || len(sessionResponse.Cookies()) != 1 {
		t.Fatal("admin bootstrap did not work on actual Quick listener")
	}
	payload := fmt.Sprintf(`{"pairing_code":%q,"passphrase":"long-local-owner-passphrase"}`, match[1])
	pair, err := http.NewRequest(http.MethodPost, "http://"+admin.AdminAddress+"/api/admin/v1/pair", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	pair.Host = "localhost:7677"
	pair.Header.Set("Origin", admin.AdminOrigin)
	pair.Header.Set("Content-Type", "application/json")
	pair.Header.Set("X-CSRF-Token", bootstrap.CSRF)
	pair.AddCookie(sessionResponse.Cookies()[0])
	paired, err := client.Do(pair)
	if err != nil {
		t.Fatal(err)
	}
	_ = paired.Body.Close()
	if paired.StatusCode != 201 || len(paired.Cookies()) == 0 {
		t.Fatalf("actual Quick pairing did not authenticate: %d", paired.StatusCode)
	}
	requests, body, err := request(admin.AdminAddress, "localhost:7677", "/api/admin/v1/requests", paired.Cookies()[len(paired.Cookies())-1])
	if err != nil || requests.StatusCode != 200 || !strings.Contains(body, `"requests":[]`) {
		t.Fatalf("authenticated request queue unavailable: err=%v", err)
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("Quick panel shutdown failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Quick panel did not shut down both services")
	}
	for _, address := range []string{admin.PublicAddress, admin.AdminAddress} {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			t.Fatalf("Quick leaked loopback listener %s: %v", address, err)
		}
		_ = listener.Close()
	}
}
