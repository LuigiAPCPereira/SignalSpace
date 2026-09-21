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
		{path: "/", contentType: "text/html; charset=utf-8", contains: "<script src=\"/admin.js\" defer></script>"},
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
			if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "connect-src 'self'") {
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
