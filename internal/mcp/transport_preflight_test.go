package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
)

func testPublicEmbedded(t *testing.T) (string, *http.Client, *string) {
	t.Helper()
	var handler http.Handler
	fault := new(string)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch *fault {
		case "wrong_resource":
			if r.URL.Path == metadataPath {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"resource": "https://other.example/mcp", "authorization_servers": []string{strings.TrimSuffix(serverOrigin(r), "/")}, "scopes_supported": []string{diagnosticScope}})
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
		case "no_tool_challenge":
			if r.URL.Path == "/mcp" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				if r.ContentLength > 56 {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"isError":false}}`))
					return
				}
			}
		}
		handler.ServeHTTP(w, r)
	}))
	server.StartTLS()
	t.Cleanup(server.Close)
	resource := server.URL + "/mcp"
	identity, err := auth.New(auth.Config{ResourceURL: resource, Issuer: server.URL, Scope: diagnosticScope, StateDir: filepath.Join(t.TempDir(), "state")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identity.Close() })
	verifier, err := NewStaticJWTVerifier(identity.PublicKey(), identity.KeyID())
	if err != nil {
		t.Fatal(err)
	}
	protected, err := NewOAuthHandler(OAuthConfig{ResourceURL: resource, Issuer: server.URL, OwnerSubject: identity.OwnerSubject()}, verifier)
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

func TestEmbeddedTransportRejectsInvalidURL(t *testing.T) {
	for _, resource := range []string{"", "http://example.com/mcp", "https://example.com/mcp?x=1", "https://example.com/other"} {
		if _, err := CheckEmbeddedTransport(context.Background(), resource, nil); err == nil {
			t.Fatalf("invalid public resource accepted: %q", resource)
		}
	}
}
