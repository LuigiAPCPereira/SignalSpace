package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminTransportServesFunctionalSameOriginUIWithoutExposingAPIData(t *testing.T) {
	handler := Handler(http.NotFoundHandler())
	for _, tc := range []struct {
		path        string
		contentType string
		contains    string
	}{
		{path: "/", contentType: "text/html; charset=utf-8", contains: "<link rel=\"stylesheet\" href=\"/admin.css\">"},
		{path: "/admin.css", contentType: "text/css; charset=utf-8", contains: ".status-badge"},
		{path: "/admin.js", contentType: "text/javascript; charset=utf-8", contains: "credentials: 'same-origin'"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tc.path, nil)
			request.Host = "localhost:7677"
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != tc.contentType {
				t.Fatalf("UI response: %d %q", response.Code, response.Header().Get("Content-Type"))
			}
			if !strings.Contains(response.Body.String(), tc.contains) {
				t.Fatalf("UI missing %q", tc.contains)
			}
			csp := response.Header().Get("Content-Security-Policy")
			if !strings.Contains(csp, "style-src 'self'") || !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "connect-src 'self'") {
				t.Fatalf("UI CSP does not permit only same-origin script/API: %q", csp)
			}
		})
	}
}

func TestAdminUIRejectsMutationWithoutSameOriginTransport(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("ignored"))
	request.Host = "localhost:7677"
	response := httptest.NewRecorder()
	Handler(http.NotFoundHandler()).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("POST without Origin reached UI: %d", response.Code)
	}
}

func TestAdminUISourcePreservesBootstrapCSRFAndRegisteredClientName(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/admin.js", nil)
	request.Host = "localhost:7677"
	response := httptest.NewRecorder()
	Handler(http.NotFoundHandler()).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("admin.js status: %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "csrf_token") || !strings.Contains(body, "csrfOverride") {
		t.Fatal("bootstrap CSRF is not preserved for pair/unlock")
	}
	if !strings.Contains(body, "item.client?.display_name") {
		t.Fatal("request UI does not use the registered client display name")
	}
	if strings.Contains(body, "localStorage") || strings.Contains(body, "sessionStorage") {
		t.Fatal("admin UI introduced Web Storage")
	}
}
