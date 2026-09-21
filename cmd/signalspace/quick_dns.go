package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const publicDNSURL = "https://dns.google/resolve"

// probeQuickPublicDNS faz somente diagnóstico numa falha DNS, sem fornecer IPs
// ao transporte OAuth nem alterar o resolvedor, o Host ou a validação TLS.
func probeQuickPublicDNS(ctx context.Context, resource string, client *http.Client, endpoint string) string {
	parsed, err := url.Parse(resource)
	if err != nil || parsed.Scheme != "https" || parsed.Path != "/mcp" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "unavailable"
	}
	host := parsed.Hostname()
	if !strings.HasSuffix(host, ".trycloudflare.com") || len(host) <= len(".trycloudflare.com") || parsed.Port() != "" {
		return "unavailable"
	}
	query := url.Values{"name": {host}, "type": {"A"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return "unavailable"
	}
	if client == nil {
		client = &http.Client{}
	}
	bounded := *client
	bounded.Timeout = 5 * time.Second
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := bounded.Do(request)
	if err != nil {
		return "unavailable"
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > 8<<10 {
		return "unavailable"
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (8<<10)+1))
	if err != nil || len(data) > 8<<10 {
		return "unavailable"
	}
	var result struct {
		Status   int `json:"Status"`
		Question []struct {
			Name string `json:"name"`
			Type int    `json:"type"`
		} `json:"Question"`
		Answer []struct {
			Name string `json:"name"`
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if json.Unmarshal(data, &result) != nil || len(result.Question) != 1 || result.Question[0].Name != host+"." || result.Question[0].Type != 1 {
		return "unavailable"
	}
	if result.Status == 3 {
		return "nxdomain"
	}
	if result.Status != 0 {
		return "unavailable"
	}
	for _, answer := range result.Answer {
		if answer.Name == host+"." && answer.Type == 1 && net.ParseIP(answer.Data).To4() != nil {
			return "resolved"
		}
	}
	return "no_a_record"
}
