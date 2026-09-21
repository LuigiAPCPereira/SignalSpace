package admin

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func forgedProxyHeaders() map[string]string {
	return map[string]string{
		"Forwarded":         "for=203.0.113.10;host=evil.example;proto=https",
		"X-Forwarded-Host":  "evil.example",
		"X-Forwarded-Proto": "https",
		"X-Forwarded-For":   "203.0.113.10",
	}
}

func applyHeaders(request *http.Request, headers map[string]string) {
	for name, value := range headers {
		request.Header.Set(name, value)
	}
}

func TestAdminTransportRejectsUnexpectedOriginsAndPaths(t *testing.T) {
	calls := 0
	handler := Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	cases := []struct {
		name, method, path, host, origin string
		want                             int
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

func TestAdminTransportRejectsForgedProxyHeadersOverLoopback(t *testing.T) {
	calls := 0
	server := httptest.NewServer(Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})))
	defer server.Close()
	client := server.Client()

	cases := []struct {
		name, method, path, host, origin, body string
		wantCode                               int
		wantCall                               bool
	}{
		{"valid_host_without_transport_credentials", http.MethodGet, "/api/admin/v1/session", "localhost:7677", "", "", http.StatusNoContent, true},
		{"invalid_host_forwarded_to_canonical", http.MethodGet, "/api/admin/v1/session", "evil.example", "", "", http.StatusForbidden, false},
		{"public_host_forwarded_to_canonical", http.MethodGet, "/api/admin/v1/session", "localhost:7676", "", "", http.StatusForbidden, false},
		{"cross_origin_forwarded_host", http.MethodGet, "/api/admin/v1/session", "localhost:7677", "https://evil.example", "", http.StatusForbidden, false},
		{"preflight_cannot_become_same_origin", http.MethodOptions, "/api/admin/v1/session", "localhost:7677", AdminOrigin, "", http.StatusForbidden, false},
		{"post_without_origin", http.MethodPost, "/api/admin/v1/pair", "localhost:7677", "", `{}`, http.StatusForbidden, false},
		{"same_origin_post_with_forged_proxy", http.MethodPost, "/api/admin/v1/pair", "localhost:7677", AdminOrigin, `{}`, http.StatusNoContent, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest(tc.method, server.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Host = tc.host
			if tc.origin != "" {
				request.Header.Set("Origin", tc.origin)
			}
			applyHeaders(request, forgedProxyHeaders())
			before := calls
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			_, _ = io.Copy(io.Discard, response.Body)
			if response.StatusCode != tc.wantCode {
				t.Fatalf("got %d, want %d", response.StatusCode, tc.wantCode)
			}
			if (calls != before) != tc.wantCall {
				t.Fatalf("API call reached transport boundary: calls=%d before=%d want=%t", calls, before, tc.wantCall)
			}
			if response.Header.Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("forged origin received a CORS grant")
			}
		})
	}
}

func TestAdminTransportProxyHeadersDoNotBypassSessionOrCSRF(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	server := httptest.NewServer(Handler(gate.Handler()))
	defer server.Close()
	client := server.Client()

	bootstrap, _, err := gate.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	pairRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/admin/v1/pair", strings.NewReader(`{"pairing_code":"`+code+`","passphrase":"`+testPassphrase+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	pairRequest.Host = "localhost:7677"
	pairRequest.Header.Set("Origin", AdminOrigin)
	pairRequest.Header.Set("Content-Type", "application/json")
	pairRequest.Header.Set("X-CSRF-Token", bootstrap.CSRF)
	applyHeaders(pairRequest, forgedProxyHeaders())
	pairRequest.AddCookie(&http.Cookie{Name: bootstrapCookie, Value: bootstrap.Cookie})
	paired, err := client.Do(pairRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer paired.Body.Close()
	if paired.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(paired.Body)
		t.Fatalf("pairing did not reach the authenticated API path: %d %s", paired.StatusCode, body)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range paired.Cookies() {
		if cookie.Name == adminCookie {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("pairing did not return an administrative session cookie")
	}

	refresh := func(cookie *http.Cookie, csrf string) (*http.Response, error) {
		request, err := http.NewRequest(http.MethodPost, server.URL+"/api/admin/v1/session/refresh", strings.NewReader("{}"))
		if err != nil {
			return nil, err
		}
		request.Host = "localhost:7677"
		request.Header.Set("Origin", AdminOrigin)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", csrf)
		applyHeaders(request, forgedProxyHeaders())
		if cookie != nil {
			request.AddCookie(cookie)
		}
		return client.Do(request)
	}

	wrongCSRF, err := refresh(sessionCookie, "forged-csrf")
	if err != nil {
		t.Fatal(err)
	}
	defer wrongCSRF.Body.Close()
	wrongBody, _ := io.ReadAll(wrongCSRF.Body)
	if wrongCSRF.StatusCode != http.StatusForbidden || !strings.Contains(string(wrongBody), `"ACCESS_DENIED"`) {
		t.Fatalf("forged proxy headers bypassed CSRF: %d %s", wrongCSRF.StatusCode, wrongBody)
	}
	if wrongCSRF.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("authenticated response granted CORS")
	}

	staleSession, err := refresh(&http.Cookie{Name: adminCookie, Value: "forged-session"}, sessionCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	defer staleSession.Body.Close()
	staleBody, _ := io.ReadAll(staleSession.Body)
	if staleSession.StatusCode != http.StatusUnauthorized || !strings.Contains(string(staleBody), `"AUTH_REQUIRED"`) {
		t.Fatalf("forged proxy headers bypassed session authentication: %d %s", staleSession.StatusCode, staleBody)
	}
}

func TestReserveListenersFailsClosedWithoutLeakingPublicPort(t *testing.T) {
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	publicAddress := probe.Addr().String()
	_ = probe.Close()
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	listeners, err := reserveListeners(publicAddress, occupied.Addr().String())
	if err == nil || listeners != nil {
		if listeners != nil {
			_ = listeners.Close()
		}
		t.Fatal("reserved public port despite admin port failure")
	}
	// Verifica se a reserva pública anterior foi liberada para não publicar um serviço incompleto.
	public, err := net.Listen("tcp4", publicAddress)
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
