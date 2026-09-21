package main

import (
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
)

func testLoopback(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

func TestQuickTerminalReservesOnlyPublicListener(t *testing.T) {
	public := testLoopback(t)
	calledBoth := false
	ports, err := reserveQuickPortsWith(false, func() (net.Listener, error) {
		return public, nil
	}, func() (*admin.Listeners, error) {
		calledBoth = true
		return nil, errors.New("should not reserve admin")
	})
	if err != nil || ports == nil || ports.Public != public || ports.Admin != nil || calledBoth {
		t.Fatalf("terminal-only reservation changed: ports=%v err=%v both=%t", ports, err, calledBoth)
	}
	if err := ports.Close(); err != nil {
		t.Fatal(err)
	}
	rebound, err := net.Listen("tcp4", public.Addr().String())
	if err != nil {
		t.Fatalf("terminal listener leaked: %v", err)
	}
	_ = rebound.Close()
}

func TestQuickPanelReservationFailsClosedAndClosesPartialListeners(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"failed_admin_bind", errors.New("occupied administrative port")},
		{"incomplete_reservation", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			public := testLoopback(t)
			calledPublic := false
			ports, err := reserveQuickPortsWith(true, func() (net.Listener, error) {
				calledPublic = true
				return nil, nil
			}, func() (*admin.Listeners, error) {
				return &admin.Listeners{Public: public}, tc.err
			})
			if err == nil || ports != nil || calledPublic {
				t.Fatalf("failed panel reservation fell back to public-only: %v %v %t", ports, err, calledPublic)
			}
			rebound, bindErr := net.Listen("tcp4", public.Addr().String())
			if bindErr != nil {
				t.Fatalf("partial public listener leaked: %v", bindErr)
			}
			_ = rebound.Close()
		})
	}
}

func TestQuickPreparedListenersServeIsolatedHTTPRouters(t *testing.T) {
	public := testLoopback(t)
	private := testLoopback(t)
	ports, err := reserveQuickPortsWith(true, func() (net.Listener, error) {
		t.Fatal("panel must never fall back to public-only reservation")
		return nil, nil
	}, func() (*admin.Listeners, error) {
		return &admin.Listeners{Public: public, Admin: private}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ports.Close()
	gate, _, err := admin.NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	publicServer := diagnosticServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	adminServer := admin.NewServer(gate.Handler())
	publicDone := make(chan error, 1)
	adminDone := make(chan error, 1)
	go func() { publicDone <- publicServer.Serve(ports.Public) }()
	go func() { adminDone <- adminServer.Serve(ports.Admin) }()
	defer func() {
		_ = publicServer.Close()
		_ = adminServer.Close()
		<-publicDone
		<-adminDone
	}()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	fetch := func(address, host, path string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, "http://"+address+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = host
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, string(body)
	}
	if status, _ := fetch(public.Addr().String(), "localhost:7676", "/api/admin/v1/session"); status != 404 {
		t.Fatalf("administrative route served on public port: %d", status)
	}
	if status, _ := fetch(private.Addr().String(), "localhost:7677", "/mcp"); status != 404 {
		t.Fatalf("public MCP route served on admin port: %d", status)
	}
	if status, body := fetch(private.Addr().String(), "localhost:7677", "/api/admin/v1/session"); status != 200 || !strings.Contains(body, `"state":"UNPAIRED"`) {
		t.Fatalf("administrative session unavailable on its own port: %d %s", status, body)
	}
	if status, _ := fetch(private.Addr().String(), "localhost:7676", "/api/admin/v1/session"); status != 403 {
		t.Fatalf("admin accepted public Host: %d", status)
	}
	if status, _ := fetch(public.Addr().String(), "localhost:7676", "/mcp"); status != 204 {
		t.Fatalf("public service did not serve its own route: %d", status)
	}
}

func TestQuickPublicListenerRejectsAdministrativeMatrix(t *testing.T) {
	public := testLoopback(t)
	handler, authorization, err := embeddedHandler("https://public.example/mcp", filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer authorization.Close()

	server := diagnosticServer(handler)
	done := make(chan error, 1)
	go func() { done <- server.Serve(public) }()
	defer func() {
		_ = server.Close()
		if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("public server: %v", err)
		}
	}()

	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   2 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.Transport.(*http.Transport).CloseIdleConnections()
	cases := []struct {
		name, method, path, host string
	}{
		{"session", http.MethodGet, "/api/admin/v1/session", "localhost:7676"},
		{"pair", http.MethodPost, "/api/admin/v1/pair", "localhost:7677"},
		{"request_detail", http.MethodGet, "/api/admin/v1/requests/abcdefghijklmnopqrstuv", "evil.example"},
		{"decision_alternative_method", http.MethodPut, "/api/admin/v1/requests/abcdefghijklmnopqrstuv/decision", "localhost:7677"},
		{"encoded_path", http.MethodGet, "/%61pi/admin/v1/session", "localhost:7677"},
		{"duplicate_slash", http.MethodOptions, "/api/admin/v1//session", "localhost:7677"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, "http://"+public.Addr().String()+tc.path, strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			req.Host = tc.host
			req.Header.Set("Cookie", "signalspace_auth=stale")
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("public listener exposed administrative path: %s %s returned %d", tc.method, tc.path, response.StatusCode)
			}
			if len(response.Cookies()) != 0 {
				t.Fatalf("public listener set administrative cookie for %s", tc.path)
			}
		})
	}
}
