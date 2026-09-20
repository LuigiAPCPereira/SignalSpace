package admin

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminTransportRejectsUnexpectedOriginsAndPaths(t *testing.T) {
	calls := 0
	handler := Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	cases := []struct {
		name, method, path, host, origin string
		want int
	}{
		{"authenticated_route_transport_only", "GET", "/api/admin/v1/session", "localhost:7677", "", 204},
		{"external_host", "GET", "/api/admin/v1/session", "evil.example", "", 403},
		{"ip_host", "GET", "/api/admin/v1/session", "127.0.0.1:7677", "", 403},
		{"public_host", "GET", "/api/admin/v1/session", "localhost:7676", "", 403},
		{"cross_origin_get", "GET", "/api/admin/v1/session", "localhost:7677", "https://evil.example", 403},
		{"missing_origin_post", "POST", "/api/admin/v1/pair", "localhost:7677", "", 403},
		{"cross_origin_post", "POST", "/api/admin/v1/pair", "localhost:7677", "https://evil.example", 403},
		{"same_origin_post", "POST", "/api/admin/v1/pair", "localhost:7677", AdminOrigin, 204},
		{"preflight", "OPTIONS", "/api/admin/v1/session", "localhost:7677", AdminOrigin, 403},
		{"public_oauth_path", "GET", "/authorize", "localhost:7677", "", 404},
		{"public_mcp_path", "POST", "/mcp", "localhost:7677", AdminOrigin, 404},
		{"duplicate_slash", "GET", "/api/admin/v1//session", "localhost:7677", "", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			before := calls
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d", w.Code, tc.want)
			}
			if tc.want != 204 && calls != before {
				t.Fatal("rejected request reached API")
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("unexpected response headers")
			}
		})
	}
}

func TestReserveListenersFailsClosedWithoutLeakingPublicPort(t *testing.T) {
	occupied, err := net.Listen("tcp4", AdminAddress)
	if err != nil {
		t.Skipf("admin port unavailable on test host: %v", err)
	}
	defer occupied.Close()
	listeners, err := ReserveListeners()
	if err == nil || listeners != nil {
		if listeners != nil {
			_ = listeners.Close()
		}
		t.Fatal("reserved public port despite admin port failure")
	}
	// Verifica se a reserva pública anterior foi liberada para não publicar um serviço incompleto.
	public, err := net.Listen("tcp4", PublicAddress)
	if err != nil {
		t.Fatalf("public port leaked after failure: %v", err)
	}
	defer public.Close()
}

func TestReserveListenersBindsExactLoopbackAddresses(t *testing.T) {
	listeners, err := ReserveListeners()
	if err != nil {
		t.Skipf("ports unavailable on test host: %v", err)
	}
	defer listeners.Close()
	if listeners.Public.Addr().String() != PublicAddress || listeners.Admin.Addr().String() != AdminAddress {
		t.Fatalf("unexpected addresses: %s and %s", listeners.Public.Addr(), listeners.Admin.Addr())
	}
}
