package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

const diagnosticScope = "signalspace:diagnostic"
const workspaceReadScope = "signalspace:workspace.read"
const metadataPath = "/.well-known/oauth-protected-resource"

var ErrInsufficientScope = errors.New("insufficient OAuth scope")

type TokenVerifier interface {
	Verify(context.Context, string, string, string, string, string) error
}

// IdentityVerifier exige um token válido e retorna identidade assinada, não
// campos fornecidos pelo cliente MCP.
type IdentityVerifier interface {
	TokenVerifier
	VerifyIdentity(context.Context, string, string, string, string, string) (VerifiedIdentity, error)
}

type OAuthConfig struct {
	ResourceURL  string
	Issuer       string
	OwnerSubject string
	// WorkspaceReader é opcional e nunca é configurado por parâmetros HTTP.
	// Uma instância sem este componente permanece exclusivamente diagnóstico.
	WorkspaceReader WorkspaceTextReader
	// WorkspaceLister é independente e só pode ser habilitado com WorkspaceReader.
	// A composição local deve injetar a mesma concessão nas duas portas.
	WorkspaceLister WorkspaceDirectoryLister
	// OnMCPEvent recebe apenas eventos de ferramentas autenticadas e nomes fixos.
	// diagnosticID é um identificador de correlação, nunca um token OAuth.
	OnMCPEvent func(method, diagnosticID string)
}

// NewOAuthHandler separa a descoberta pública da autorização obrigatória no MCP.
// O cliente de autenticação é injetado; não existe fallback para o token local.
func NewOAuthHandler(config OAuthConfig, verifier TokenVerifier) (http.Handler, error) {
	if verifier == nil {
		return nil, errors.New("OAuth token verifier is required")
	}
	// O proprietário deve ser identificado por sub exato emitido pelo provedor confiável.
	if len(config.OwnerSubject) == 0 || len(config.OwnerSubject) > 512 || strings.TrimSpace(config.OwnerSubject) != config.OwnerSubject || strings.IndexFunc(config.OwnerSubject, unicode.IsControl) >= 0 {
		return nil, errors.New("OAuth owner subject must be configured explicitly")
	}
	resource, err := parseSecureURL(config.ResourceURL)
	if err != nil || resource.Path != "/mcp" || resource.RawPath != "" || resource.String() != config.ResourceURL {
		return nil, errors.New("resource URL must be the canonical HTTPS /mcp endpoint")
	}
	issuer, err := parseSecureURL(config.Issuer)
	if err != nil || issuer.String() != config.Issuer {
		return nil, errors.New("OAuth issuer must be an absolute HTTPS URL")
	}
	if config.WorkspaceLister != nil && config.WorkspaceReader == nil {
		return nil, errors.New("workspace directory listing requires workspace reader")
	}
	var identityVerifier IdentityVerifier
	if config.WorkspaceReader != nil {
		identityVerifier, _ = verifier.(IdentityVerifier)
		if identityVerifier == nil {
			return nil, errors.New("workspace reader requires verified OAuth client identity")
		}
	}
	origin := "https://" + resource.Host
	metadataURL := origin + metadataPath
	challenge := fmt.Sprintf(`Bearer resource_metadata="%s", scope="%s"`, metadataURL, diagnosticScope)
	scopes := []string{diagnosticScope}
	if config.WorkspaceReader != nil {
		scopes = append(scopes, workspaceReadScope)
	}
	metadata := map[string]any{
		"resource":              config.ResourceURL,
		"authorization_servers": []string{config.Issuer},
		"scopes_supported":      scopes,
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
			if !toolAuthChallenge(w, r, challenge+`, error="invalid_token", error_description="Authentication required to use this tool"`) {
				unauthorized(w, http.StatusUnauthorized, challenge)
			}
			return
		}
		if err := verifier.Verify(r.Context(), strings.TrimPrefix(bearer, "Bearer "), config.Issuer, config.ResourceURL, diagnosticScope, config.OwnerSubject); err != nil {
			if errors.Is(err, ErrInsufficientScope) {
				withError := challenge + `, error="insufficient_scope", error_description="Diagnostic scope is required"`
				if !toolAuthChallenge(w, r, withError) {
					unauthorized(w, http.StatusForbidden, withError)
				}
			} else {
				withError := challenge + `, error="invalid_token", error_description="Invalid access token"`
				if !toolAuthChallenge(w, r, withError) {
					unauthorized(w, http.StatusUnauthorized, withError)
				}
			}
			return
		}
		var readAccess *readToolAccess
		if config.WorkspaceReader != nil {
			accessToken := strings.TrimPrefix(bearer, "Bearer ")
			readAccess = &readToolAccess{
				reader: config.WorkspaceReader,
				lister: config.WorkspaceLister,
				verify: func(ctx context.Context) (VerifiedIdentity, error) {
					identity, err := identityVerifier.VerifyIdentity(ctx, accessToken, config.Issuer, config.ResourceURL, workspaceReadScope, config.OwnerSubject)
					if err != nil || identity.OwnerSubject != config.OwnerSubject || !embeddedClientID.MatchString(identity.ClientID) {
						if err == nil {
							err = errInvalidToken
						}
						return VerifiedIdentity{}, err
					}
					return identity, nil
				},
				challenge: fmt.Sprintf(`Bearer resource_metadata="%s", scope="%s"`, metadataURL, workspaceReadScope),
			}
		}
		serveMCP(w, r, "oauth_diagnostic", config.OnMCPEvent, readAccess)
	}), nil
}

// toolAuthChallenge devolve o desafio MCP apenas para chamadas reconhecidas da ferramenta.
// Nenhum argumento é executado ou considerado confiável antes da autorização.
func toolAuthChallenge(w http.ResponseWriter, r *http.Request, challenge string) bool {
	if r.Method != http.MethodPost || strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])) != "application/json" || r.ContentLength > maxBodyBytes {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil || len(data) > maxBodyBytes {
		return false
	}
	var msg request
	if json.Unmarshal(data, &msg) != nil || msg.JSONRPC != "2.0" || msg.Method != "tools/call" || len(msg.ID) == 0 || !validObject(msg.Params) {
		return false
	}
	id, ok := validID(msg.ID)
	if !ok {
		return false
	}
	var params struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(msg.Params, &params) != nil || params.Name != toolName {
		return false
	}
	reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []any{map[string]any{"type": "text", "text": "Authentication required to use this tool."}},
		"_meta":   map[string]any{"mcp/www_authenticate": []string{challenge}},
		"isError": true,
	}})
	return true
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
