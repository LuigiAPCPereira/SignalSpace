package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
)

const maxMetadataBytes = 64 * 1024

// OAuthPreflightReport registra apenas capacidades publicadas, nunca credenciais.
// A aprovação do provedor não comprova autorização de usuário nem compatibilidade com ChatGPT.
type OAuthPreflightReport struct {
	MetadataURL           string
	AuthorizationEndpoint string
	TokenEndpoint         string
	Registration          string
	JWKSURL               string
}

type authorizationServerMetadata struct {
	Issuer                   string   `json:"issuer"`
	AuthorizationEndpoint    string   `json:"authorization_endpoint"`
	TokenEndpoint            string   `json:"token_endpoint"`
	JWKSURI                  string   `json:"jwks_uri"`
	RegistrationEndpoint     string   `json:"registration_endpoint"`
	ClientIDMetadataDocument bool     `json:"client_id_metadata_document_supported"`
	CodeChallengeMethods     []string `json:"code_challenge_methods_supported"`
	ResponseTypes            []string `json:"response_types_supported"`
	GrantTypes               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethods []string `json:"token_endpoint_auth_methods_supported"`
}

// CheckOAuthProvider consulta apenas metadados de URLs derivadas do emissor fixado.
// Não solicita tokens, credenciais ou consentimento, nem acessa um recurso MCP público.
func CheckOAuthProvider(ctx context.Context, issuer, jwksURL string, client *http.Client) (OAuthPreflightReport, error) {
	var report OAuthPreflightReport
	parsed, err := parseSecureURL(issuer)
	if err != nil || parsed.String() != issuer {
		return report, errors.New("OAuth issuer must be a canonical HTTPS URL")
	}
	jwks, err := parseSecureURL(jwksURL)
	if err != nil || jwks.String() != jwksURL {
		return report, errors.New("JWKS URL must be a canonical HTTPS URL")
	}
	if client == nil {
		client = &http.Client{}
	}
	// Não alterar o cliente recebido; preservar seu transporte TLS, mas proibir redirecionamentos.
	boundedClient := *client
	boundedClient.Timeout = 5 * time.Second
	boundedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	origin := "https://" + parsed.Host
	issuerPath := strings.TrimSuffix(parsed.EscapedPath(), "/")
	locations := []string{
		origin + "/.well-known/oauth-authorization-server" + issuerPath,
		origin + issuerPath + "/.well-known/openid-configuration",
	}
	var metadata authorizationServerMetadata
	for i, location := range locations {
		metadata, err = fetchAuthorizationMetadata(ctx, &boundedClient, location)
		if errors.Is(err, errMetadataNotFound) && i == 0 {
			continue
		}
		if err != nil {
			return report, fmt.Errorf("authorization metadata: %w", err)
		}
		report.MetadataURL = location
		break
	}
	if report.MetadataURL == "" {
		return report, errors.New("authorization server metadata not found")
	}
	if metadata.Issuer != issuer {
		return report, errors.New("authorization metadata issuer does not match configured issuer")
	}
	if err := validateEndpoint(metadata.AuthorizationEndpoint); err != nil {
		return report, fmt.Errorf("authorization endpoint: %w", err)
	}
	if err := validateEndpoint(metadata.TokenEndpoint); err != nil {
		return report, fmt.Errorf("token endpoint: %w", err)
	}
	if err := validateEndpoint(metadata.JWKSURI); err != nil || metadata.JWKSURI != jwksURL {
		return report, errors.New("authorization metadata JWKS URI does not match configured JWKS URL")
	}
	if !contains(metadata.ResponseTypes, "code") || (len(metadata.GrantTypes) != 0 && !contains(metadata.GrantTypes, "authorization_code")) {
		return report, errors.New("authorization code flow is not advertised")
	}
	if !contains(metadata.CodeChallengeMethods, "S256") {
		return report, errors.New("PKCE S256 support is not advertised")
	}
	if metadata.ClientIDMetadataDocument {
		report.Registration = "cimd"
	} else if metadata.RegistrationEndpoint != "" {
		if err := validateEndpoint(metadata.RegistrationEndpoint); err != nil {
			return report, fmt.Errorf("registration endpoint: %w", err)
		}
		report.Registration = "dcr"
	} else {
		report.Registration = "pre_registered_client_required"
	}
	// Para CIMD, o ChatGPT precisa de um método de autenticação de cliente aceito.
	if report.Registration == "cimd" && !contains(metadata.TokenEndpointAuthMethods, "none") && !contains(metadata.TokenEndpointAuthMethods, "private_key_jwt") {
		return report, errors.New("CIMD requires a token endpoint that advertises none or private_key_jwt")
	}

	verifier, err := NewJWKSVerifier(jwksURL)
	if err != nil {
		return report, err
	}
	verifier.client = &boundedClient
	if _, err := verifier.publicKeys(ctx); err != nil {
		return report, fmt.Errorf("JWKS verification keys: %w", err)
	}
	report.AuthorizationEndpoint = metadata.AuthorizationEndpoint
	report.TokenEndpoint = metadata.TokenEndpoint
	report.JWKSURL = jwksURL
	return report, nil
}

var errMetadataNotFound = errors.New("metadata not found")

func fetchAuthorizationMetadata(ctx context.Context, client *http.Client, location string) (authorizationServerMetadata, error) {
	var metadata authorizationServerMetadata
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return metadata, err
	}
	res, err := client.Do(req)
	if err != nil {
		return metadata, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return metadata, errMetadataNotFound
	}
	if res.StatusCode != http.StatusOK {
		return metadata, fmt.Errorf("HTTP status %d", res.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return metadata, errors.New("metadata must use application/json")
	}
	if res.ContentLength > maxMetadataBytes {
		return metadata, errors.New("metadata exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxMetadataBytes+1))
	if err != nil || len(data) > maxMetadataBytes {
		return metadata, errors.New("metadata exceeds size limit or is unavailable")
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return metadata, errors.New("invalid authorization metadata JSON")
	}
	return metadata, nil
}

func validateEndpoint(raw string) error {
	parsed, err := parseSecureURL(raw)
	if err != nil || parsed.String() != raw {
		return errors.New("expected canonical HTTPS URL without credentials, query or fragment")
	}
	return nil
}

func contains(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}
