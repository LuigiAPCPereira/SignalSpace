package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRegistrationOptionalClientNameDoesNotImpersonateChatGPT(t *testing.T) {
	s, h, _ := startAuth(t)
	var rejected []string
	s.config.OnRegistrationFailure = func(reason string) { rejected = append(rejected, reason) }
	w := invoke(h, "POST", "/register", `{"redirect_uris":["https://chatgpt.com/connector_platform_oauth_redirect"],"token_endpoint_auth_method":"none","software_id":"untrusted"}`, "application/json", nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("optional client name rejected: status=%d body=%s", w.Code, w.Body.String())
	}
	var registered struct {
		ClientID   string `json:"client_id"`
		ClientName string `json:"client_name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &registered); err != nil {
		t.Fatal(err)
	}
	if registered.ClientName != "Cliente sem nome informado" || registered.ClientID == "" || s.clients[registered.ClientID].Name != registered.ClientName || len(rejected) != 0 {
		t.Fatalf("untrusted name was misrepresented: %+v; failures=%v", registered, rejected)
	}
}

func TestRegistrationNegotiatesUnsupportedRefreshGrantOut(t *testing.T) {
	_, h, _ := startAuth(t)
	w := invoke(h, "POST", "/register", `{"client_name":"ChatGPT","redirect_uris":["https://chatgpt.com/connector/oauth/specific-callback"],"grant_types":["refresh_token","authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, "application/json", nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("registration with optional refresh grant rejected: %d %s", w.Code, w.Body.String())
	}
	var registered struct {
		GrantTypes []string `json:"grant_types"`
		Redirects  []string `json:"redirect_uris"`
		AuthMethod string   `json:"token_endpoint_auth_method"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &registered); err != nil {
		t.Fatal(err)
	}
	if len(registered.GrantTypes) != 1 || registered.GrantTypes[0] != "authorization_code" || registered.AuthMethod != "none" || len(registered.Redirects) != 1 || registered.Redirects[0] != "https://chatgpt.com/connector/oauth/specific-callback" {
		t.Fatalf("unsupported grant advertised or callback lost: %+v", registered)
	}
}

func TestRegistrationRejectsUnsupportedMetadataWithSafeReason(t *testing.T) {
	tests := []struct {
		name, body, kind, reason string
	}{
		{"unsupported_grant", `{"client_name":"ChatGPT","redirect_uris":["` + callback + `"],"grant_types":["refresh_token"]}`, "invalid_client_metadata", "unsupported_grant_types"},
		{"duplicate_grant", `{"client_name":"ChatGPT","redirect_uris":["` + callback + `"],"grant_types":["authorization_code","authorization_code"]}`, "invalid_client_metadata", "unsupported_grant_types"},
		{"unsupported_auth", `{"client_name":"ChatGPT","redirect_uris":["` + callback + `"],"token_endpoint_auth_method":"client_secret_basic"}`, "invalid_client_metadata", "unsupported_token_auth_method"},
		{"unsupported_response", `{"client_name":"ChatGPT","redirect_uris":["` + callback + `"],"response_types":["token"]}`, "invalid_client_metadata", "unsupported_response_types"},
		{"forbidden_redirect", `{"client_name":"ChatGPT","redirect_uris":["https://malicious.example/callback"]}`, "invalid_redirect_uri", "redirect_uri_not_allowed"},
		{"missing_redirect", `{"client_name":"ChatGPT"}`, "invalid_client_metadata", "invalid_redirect_uris"},
		{"bad_name", `{"client_name":"client\nname","redirect_uris":["` + callback + `"]}`, "invalid_client_metadata", "invalid_client_name"},
		{"bad_json", `{invalid`, "invalid_client_metadata", "malformed_json"},
		{"multiple_json", `{} {}`, "invalid_client_metadata", "multiple_json_values"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, h, _ := startAuth(t)
			var failures []string
			s.config.OnRegistrationFailure = func(reason string) { failures = append(failures, reason) }
			w := invoke(h, "POST", "/register", tc.body, "application/json", nil)
			if w.Code != 400 {
				t.Fatalf("rejected with status %d: %s", w.Code, w.Body.String())
			}
			var response struct {
				Error       string `json:"error"`
				Description string `json:"error_description"`
			}
			if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Error != tc.kind || response.Description != tc.reason || len(failures) != 1 || failures[0] != tc.reason || len(s.clients) != 0 {
				t.Fatalf("invalid rejection semantics: %+v, failures=%v, clients=%d", response, failures, len(s.clients))
			}
			if strings.Contains(w.Body.String(), "malicious.example") || strings.Contains(w.Body.String(), "ChatGPT") {
				t.Fatal("request metadata leaked in error response")
			}
		})
	}
}
