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

// TransportReport descreve apenas os endpoints observados, sem autenticar o ChatGPT.
type TransportReport struct {
	ResourceURL string
	Issuer      string
	MetadataURL string
}

type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

// CheckEmbeddedTransport testa o HTTPS público sem credenciais e sem modificar estado.
// O teste não registra clientes, solicita autorização nem conclui uma conexão ChatGPT.
func CheckEmbeddedTransport(ctx context.Context, resourceURL string, client *http.Client) (TransportReport, error) {
	var report TransportReport
	resource, err := parseSecureURL(resourceURL)
	if err != nil || resource.Path != "/mcp" || resource.RawPath != "" || resource.String() != resourceURL {
		return report, errors.New("transport diagnostic requires a canonical HTTPS /mcp resource URL")
	}
	origin := "https://" + resource.Host
	metadataURL := origin + metadataPath
	if client == nil {
		client = &http.Client{}
	}
	bounded := *client
	bounded.Timeout = 5 * time.Second
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	var protected protectedResourceMetadata
	if err := fetchTransportJSON(ctx, &bounded, metadataURL, &protected); err != nil {
		return report, fmt.Errorf("protected resource metadata: %w", err)
	}
	if protected.Resource != resourceURL || len(protected.AuthorizationServers) != 1 || protected.AuthorizationServers[0] != origin || !contains(protected.ScopesSupported, diagnosticScope) {
		return report, errors.New("protected resource metadata does not match the configured resource, issuer or diagnostic scope")
	}
	provider, err := CheckOAuthProvider(ctx, origin, origin+"/oauth/jwks", &bounded)
	if err != nil {
		return report, fmt.Errorf("embedded authorization discovery: %w", err)
	}
	if provider.Registration != "dcr" || provider.AuthorizationEndpoint != origin+"/authorize" || provider.TokenEndpoint != origin+"/token" || provider.MetadataURL != origin+"/.well-known/oauth-authorization-server" {
		return report, errors.New("authorization server endpoints do not match embedded OAuth")
	}

	// Um cliente sem token deve receber o desafio HTTP para descobrir o OAuth.
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	response, err := transportPost(ctx, &bounded, resourceURL, body)
	if err != nil {
		return report, err
	}
	if response.StatusCode != http.StatusUnauthorized || (!strings.Contains(response.Header.Get("WWW-Authenticate"), `resource_metadata="`+metadataURL+`"`) || !strings.Contains(response.Header.Get("WWW-Authenticate"), `scope="`+diagnosticScope+`"`)) {
		response.Body.Close()
		return report, errors.New("unauthenticated MCP request did not advertise the expected OAuth metadata")
	}
	response.Body.Close()

	// O desafio no resultado MCP é exigido para iniciar a vinculação da ferramenta.
	response, err = transportPost(ctx, &bounded, resourceURL, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"connection_diagnostic","arguments":{}}}`)
	if err != nil {
		return report, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > maxMetadataBytes {
		return report, fmt.Errorf("MCP tool authorization challenge returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxMetadataBytes+1))
	if err != nil || len(data) > maxMetadataBytes {
		return report, errors.New("MCP authorization challenge exceeded response limit")
	}
	var challenge struct {
		Result struct {
			IsError bool `json:"isError"`
			Meta    struct {
				Challenges []string `json:"mcp/www_authenticate"`
			} `json:"_meta"`
		} `json:"result"`
	}
	if json.Unmarshal(data, &challenge) != nil || !challenge.Result.IsError || len(challenge.Result.Meta.Challenges) != 1 || !strings.Contains(challenge.Result.Meta.Challenges[0], `resource_metadata="`+metadataURL+`"`) || (!strings.Contains(challenge.Result.Meta.Challenges[0], `error="invalid_token"`) || !strings.Contains(challenge.Result.Meta.Challenges[0], `error_description="`)) {
		return report, errors.New("MCP tool did not return the expected OAuth authentication challenge")
	}
	return TransportReport{ResourceURL: resourceURL, Issuer: origin, MetadataURL: metadataURL}, nil
}

func transportPost(ctx context.Context, client *http.Client, resourceURL, body string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, resourceURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("MCP-Protocol-Version", protocolVersion)
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("public MCP request: %w", err)
	}
	return response, nil
}

func fetchTransportJSON(ctx context.Context, client *http.Client, location string, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > maxMetadataBytes {
		return fmt.Errorf("unexpected HTTP status %d or oversized metadata", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("expected JSON metadata")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxMetadataBytes+1))
	if err != nil || len(data) > maxMetadataBytes {
		return errors.New("metadata exceeded response limit")
	}
	if json.Unmarshal(data, result) != nil {
		return errors.New("invalid JSON metadata")
	}
	return nil
}
