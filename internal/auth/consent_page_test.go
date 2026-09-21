package auth

import (
	"net/http"
	"strings"
	"testing"
)

func TestConsentPageUsesSameOriginPollingScriptWithStrictScriptCSP(t *testing.T) {
	_, handler, events := startAuth(t)
	clientID := register(t, handler)
	page, _, _ := requestConsent(t, handler, clientID)
	request := <-events

	csp := page.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "script-src 'unsafe-inline'") {
		t.Fatalf("consent CSP permits unsafe scripts: %q", csp)
	}
	if !strings.Contains(page.Body.String(), `<main id="consent" data-request-id="`+request.ID+`"`) || !strings.Contains(page.Body.String(), `<script src="/authorize/consent.js" defer></script>`) {
		t.Fatalf("consent page did not include request-bound polling bootstrap: %s", page.Body.String())
	}

	script := invoke(handler, http.MethodGet, consentScriptPath, "", "", nil)
	if script.Code != http.StatusOK || script.Header().Get("Content-Type") != "application/javascript; charset=utf-8" || script.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("consent script response: %d %v", script.Code, script.Header())
	}
	for _, marker := range []string{
		`/authorize/status?request_id=`,
		`credentials: "same-origin"`,
		`"PENDING"`,
		`"APPROVED"`,
		`"DENIED"`,
		`"EXPIRED"`,
		`Retry-After`,
		`Não foi possível consultar o estado`,
	} {
		if !strings.Contains(script.Body.String(), marker) {
			t.Fatalf("consent script missing marker %q", marker)
		}
	}
	for _, secretMarker := range []string{"signalspace_auth", "csrf", "access_token", "localStorage", "sessionStorage"} {
		if strings.Contains(script.Body.String(), secretMarker) {
			t.Fatalf("consent script contains sensitive client-side marker %q", secretMarker)
		}
	}

	post := invoke(handler, http.MethodPost, consentScriptPath, "", "", nil)
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("consent script mutation accepted: %d", post.Code)
	}
}
