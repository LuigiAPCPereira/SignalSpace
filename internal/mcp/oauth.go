package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const diagnosticScope = "signalspace:diagnostic"
const metadataPath = "/.well-known/oauth-protected-resource"

var ErrInsufficientScope = errors.New("insufficient OAuth scope")

type TokenVerifier interface {
	Verify(context.Context, string, string, string, string) error
}

type OAuthConfig struct {
	ResourceURL string
	Issuer      string
}

// NewOAuthHandler separa a descoberta pública da autorização obrigatória no MCP.
// O cliente de autenticação é injetado; não existe fallback para o token local.
func NewOAuthHandler(config OAuthConfig, verifier TokenVerifier) (http.Handler, error) {
	if verifier == nil {
		return nil, errors.New("OAuth token verifier is required")
	}
	resource, err := parseSecureURL(config.ResourceURL)
	if err != nil || resource.Path != "/mcp" || resource.RawPath != "" || resource.String() != config.ResourceURL {
		return nil, errors.New("resource URL must be the canonical HTTPS /mcp endpoint")
	}
	issuer, err := parseSecureURL(config.Issuer)
	if err != nil || issuer.String() != config.Issuer {
		return nil, errors.New("OAuth issuer must be an absolute HTTPS URL")
	}
	origin := "https://" + resource.Host
	metadataURL := origin + metadataPath
	challenge := fmt.Sprintf(`Bearer resource_metadata="%s", scope="%s"`, metadataURL, diagnosticScope)
	metadata := map[string]any{
		"resource":              config.ResourceURL,
		"authorization_servers": []string{config.Issuer},
		"scopes_supported":      []string{diagnosticScope},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Nunca confiar em X-Forwarded-Host ou X-Forwarded-Proto enviados pelo cliente.
		if r.Host != resource.Host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if (r.URL.Path == metadataPath || r.URL.Path == metadataPath+"/mcp") && r.URL.RawQuery == "" {
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			_ = json.NewEncoder(w).Encode(metadata)
			return
		}
		if r.URL.Path != "/mcp" || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		bearer := r.Header.Get("Authorization")
		if !strings.HasPrefix(bearer, "Bearer ") || len(bearer) <= len("Bearer ") {
			unauthorized(w, http.StatusUnauthorized, challenge)
			return
		}
		if err := verifier.Verify(r.Context(), strings.TrimPrefix(bearer, "Bearer "), config.Issuer, config.ResourceURL, diagnosticScope); err != nil {
			if errors.Is(err, ErrInsufficientScope) {
				unauthorized(w, http.StatusForbidden, challenge+`, error="insufficient_scope"`)
			} else {
				unauthorized(w, http.StatusUnauthorized, challenge+`, error="invalid_token"`)
			}
			return
		}
		serveMCP(w, r, "oauth_diagnostic")
	}), nil
}

func parseSecureURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return nil, errors.New("expected an absolute HTTPS URL without credentials, query or fragment")
	}
	return u, nil
}

func unauthorized(w http.ResponseWriter, status int, challenge string) {
	w.Header().Set("WWW-Authenticate", challenge)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
}
