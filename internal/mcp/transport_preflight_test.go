package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
)

func testPublicEmbedded(t *testing.T) (string, *http.Client, *string) {
	return testPublicEmbeddedMode(t, ScopeDiagnostic)
}

func testPublicEmbeddedMode(t *testing.T, scope string) (string, *http.Client, *string) {
	t.Helper()
	var handler http.Handler
	fault := new(string)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := serverOrigin(r)
		switch *fault {
		case "wrong_resource":
			if r.URL.Path == metadataPath {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"resource": "https://other.example/mcp", "authorization_servers": []string{strings.TrimSuffix(origin, "/")}, "scopes_supported": []string{scope}})
				return
			}
		case "issuer_mismatch":
			if r.URL.Path == metadataPath {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"resource": origin + "/mcp", "authorization_servers": []string{"https://other.example"}, "scopes_supported": []string{scope}})
				return
			}
		case "missing_scope":
			if r.URL.Path == metadataPath {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"resource": origin + "/mcp", "authorization_servers": []string{strings.TrimSuffix(origin, "/")}, "scopes_supported": []string{"unrelated:scope"}})
				return
			}
		case "extra_scope":
			if r.URL.Path == metadataPath {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"resource": origin + "/mcp", "authorization_servers": []string{strings.TrimSuffix(origin, "/")}, "scopes_supported": []string{scope, "unrelated:scope"}})
				return
			}
		case "auth_server_missing_scope":
			if r.URL.Path == "/.well-known/oauth-authorization-server" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"issuer":                                origin,
					"authorization_endpoint":                origin + "/authorize",
					"token_endpoint":                        origin + "/token",
					"registration_endpoint":                 origin + "/register",
					"jwks_uri":                              origin + "/oauth/jwks",
					"response_types_supported":              []string{"code"},
					"grant_types_supported":                 []string{"authorization_code"},
					"code_challenge_methods_supported":      []string{"S256"},
					"token_endpoint_auth_methods_supported": []string{"none"},
					"scopes_supported":                      []string{"unrelated:scope"},
				})
				return
			}
		case "auth_server_extra_scope":
			if r.URL.Path == "/.well-known/oauth-authorization-server" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"issuer":                                origin,
					"authorization_endpoint":                origin + "/authorize",
					"token_endpoint":                        origin + "/token",
					"registration_endpoint":                 origin + "/register",
					"jwks_uri":                              origin + "/oauth/jwks",
					"response_types_supported":              []string{"code"},
					"grant_types_supported":                 []string{"authorization_code"},
					"code_challenge_methods_supported":      []string{"S256"},
					"token_endpoint_auth_methods_supported": []string{"none"},
					"scopes_supported":                      []string{scope, "unrelated:scope"},
				})
				return
			}
		case "redirect":
			if r.URL.Path == metadataPath {
				http.Redirect(w, r, "https://other.example/", http.StatusFound)
				return
			}
		case "no_http_challenge":
			if r.URL.Path == "/mcp" {
				w.WriteHeader(http.StatusOK)
				return
			}
		case "wrong_http_challenge_scope":
			if r.URL.Path == "/mcp" && r.Header.Get("Authorization") == "" {
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+origin+metadataPath+`", scope="unrelated:scope"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		case "swapped_http_challenge_scope":
			if r.URL.Path == "/mcp" && r.Header.Get("Authorization") == "" {
				swapped := ScopeDiagnostic
				if scope == ScopeDiagnostic {
					swapped = ScopeProgramming
				}
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+origin+metadataPath+`", scope="`+swapped+`"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		case "no_tool_challenge":
			if r.URL.Path == "/mcp" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				if r.ContentLength > 56 {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"isError":false}}`))
					return
				}
			}
		case "wrong_tool_challenge_scope":
			if r.URL.Path == "/mcp" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				if r.ContentLength > 56 {
					w.Header().Set("Content-Type", "application/json")
					challenge := fmt.Sprintf(`Bearer resource_metadata="%s%s", scope="unrelated:scope", error="invalid_token", error_description="Authentication required to use this tool"`, origin, metadataPath)
					_, _ = w.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"result":{"isError":true,"_meta":{"mcp/www_authenticate":[%q]}}}`, challenge)))
					return
				}
			}
		case "swapped_tool_challenge_scope":
			if r.URL.Path == "/mcp" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				if r.ContentLength > 56 {
					swapped := ScopeDiagnostic
					if scope == ScopeDiagnostic {
						swapped = ScopeProgramming
					}
					w.Header().Set("Content-Type", "application/json")
					challenge := fmt.Sprintf(`Bearer resource_metadata="%s%s", scope="%s", error="invalid_token", error_description="Authentication required to use this tool"`, origin, metadataPath, swapped)
					_, _ = w.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"result":{"isError":true,"_meta":{"mcp/www_authenticate":[%q]}}}`, challenge)))
					return
				}
			}
		}
		handler.ServeHTTP(w, r)
	}))
	server.StartTLS()
	t.Cleanup(server.Close)
	resource := server.URL + "/mcp"
	authCfg := auth.Config{ResourceURL: resource, Issuer: server.URL, Scope: scope, StateDir: filepath.Join(t.TempDir(), "state")}
	if scope == ScopeProgramming {
		authCfg.CompositionScope = ScopeProgramming
		authCfg.EnableRefreshTokens = true
	}
	identity, err := auth.New(authCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identity.Close() })
	verifier, err := NewStaticJWTVerifier(identity.PublicKey(), identity.KeyID())
	if err != nil {
		t.Fatal(err)
	}
	mcpCfg := OAuthConfig{ResourceURL: resource, Issuer: server.URL, OwnerSubject: identity.OwnerSubject()}
	if scope == ScopeProgramming {
		mcpCfg.discoverProgrammingTools = true
		mcpCfg.programmingOAuthV2 = true
	}
	protected, err := NewOAuthHandler(mcpCfg, verifier)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", protected)
	mux.Handle(metadataPath, protected)
	for _, path := range []string{"/.well-known/oauth-authorization-server", "/oauth/jwks", "/register", "/authorize", "/authorize/complete", "/token"} {
		mux.Handle(path, identity.Handler())
	}
	handler = mux
	return resource, server.Client(), fault
}

func serverOrigin(r *http.Request) string { return "https://" + r.Host }

func TestEmbeddedTransportPublicContract(t *testing.T) {
	resource, client, fault := testPublicEmbedded(t)
	report, err := CheckEmbeddedTransport(context.Background(), resource, client)
	if err != nil || report.ResourceURL != resource || report.Issuer != strings.TrimSuffix(resource, "/mcp") {
		t.Fatalf("valid remote public contract rejected: %+v %v", report, err)
	}
	for _, name := range []string{"wrong_resource", "redirect", "no_http_challenge", "no_tool_challenge"} {
		t.Run(name, func(t *testing.T) {
			*fault = name
			defer func() { *fault = "" }()
			if _, err := CheckEmbeddedTransport(context.Background(), resource, client); err == nil {
				t.Fatal("unsafe public response passed transport diagnostic")
			}
		})
	}
	if _, err := CheckEmbeddedTransport(context.Background(), resource, nil); err == nil {
		t.Fatal("untrusted HTTPS certificate accepted")
	}
}

func TestEmbeddedTransportCompositionMatrix(t *testing.T) {
	diagResource, diagClient, diagFault := testPublicEmbeddedMode(t, ScopeDiagnostic)
	progResource, progClient, progFault := testPublicEmbeddedMode(t, ScopeProgramming)

	// Diagnostic metadata + diagnostic expected: PASS
	diagReport, err := CheckEmbeddedTransportForScope(context.Background(), diagResource, ScopeDiagnostic, diagClient)
	if err != nil || diagReport.ResourceURL != diagResource {
		t.Fatalf("diagnostic metadata + diagnostic expected failed: %v", err)
	}

	// Programming metadata + programming expected: PASS
	progReport, err := CheckEmbeddedTransportForScope(context.Background(), progResource, ScopeProgramming, progClient)
	if err != nil || progReport.ResourceURL != progResource {
		t.Fatalf("programming metadata + programming expected failed: %v", err)
	}

	// Programming metadata + diagnostic expected: FAIL
	if _, err := CheckEmbeddedTransportForScope(context.Background(), progResource, ScopeDiagnostic, progClient); err == nil {
		t.Fatal("programming metadata + diagnostic expected should have failed")
	}

	// Diagnostic metadata + programming expected: FAIL
	if _, err := CheckEmbeddedTransportForScope(context.Background(), diagResource, ScopeProgramming, diagClient); err == nil {
		t.Fatal("diagnostic metadata + programming expected should have failed")
	}

	// Unsupported composition scope: FAIL
	for _, invalidScope := range []string{"", "unsupported", "signalspace:workspace.read"} {
		if _, err := CheckEmbeddedTransportForScope(context.Background(), diagResource, invalidScope, diagClient); err == nil {
			t.Fatalf("unsupported scope %q was accepted", invalidScope)
		}
	}

	// Matrix of failures and extra scope for both modes
	modes := []struct {
		name     string
		resource string
		client   *http.Client
		fault    *string
		scope    string
	}{
		{"diagnostic", diagResource, diagClient, diagFault, ScopeDiagnostic},
		{"programming", progResource, progClient, progFault, ScopeProgramming},
	}

	for _, m := range modes {
		// Extra unrelated scope on protected resource: PASS
		t.Run(m.name+"_extra_scope", func(t *testing.T) {
			*m.fault = "extra_scope"
			defer func() { *m.fault = "" }()
			report, err := CheckEmbeddedTransportForScope(context.Background(), m.resource, m.scope, m.client)
			if err != nil || report.ResourceURL != m.resource {
				t.Fatalf("extra scope rejected: %v", err)
			}
		})

		// Extra unrelated scope on authorization server metadata: PASS
		t.Run(m.name+"_auth_server_extra_scope", func(t *testing.T) {
			*m.fault = "auth_server_extra_scope"
			defer func() { *m.fault = "" }()
			report, err := CheckEmbeddedTransportForScope(context.Background(), m.resource, m.scope, m.client)
			if err != nil || report.ResourceURL != m.resource {
				t.Fatalf("auth server extra scope rejected: %v", err)
			}
		})

		for _, failureFault := range []string{
			"wrong_resource",
			"issuer_mismatch",
			"missing_scope",
			"auth_server_missing_scope",
			"wrong_http_challenge_scope",
			"swapped_http_challenge_scope",
			"wrong_tool_challenge_scope",
			"swapped_tool_challenge_scope",
		} {
			t.Run(m.name+"_"+failureFault, func(t *testing.T) {
				*m.fault = failureFault
				defer func() { *m.fault = "" }()
				if _, err := CheckEmbeddedTransportForScope(context.Background(), m.resource, m.scope, m.client); err == nil {
					t.Fatalf("fault %s passed transport preflight for %s", failureFault, m.name)
				}
			})
		}
	}
}

func TestEmbeddedTransportRejectsInvalidURL(t *testing.T) {
	for _, resource := range []string{"", "http://example.com/mcp", "https://example.com/mcp?x=1", "https://example.com/other"} {
		if _, err := CheckEmbeddedTransport(context.Background(), resource, nil); err == nil {
			t.Fatalf("invalid public resource accepted: %q", resource)
		}
		if _, err := CheckEmbeddedTransportForScope(context.Background(), resource, ScopeDiagnostic, nil); err == nil {
			t.Fatalf("invalid public resource accepted for diagnostic scope: %q", resource)
		}
		if _, err := CheckEmbeddedTransportForScope(context.Background(), resource, ScopeProgramming, nil); err == nil {
			t.Fatalf("invalid public resource accepted for programming scope: %q", resource)
		}
	}
}
