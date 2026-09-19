package mcp

import (
	"net/http"
	"strings"
	"testing"
)

func TestOAuthToolLevelAuthorizationChallenge(t *testing.T) {
	key, _, _, server := setupOAuth(t)
	call := `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"connection_diagnostic","arguments":{}}}`
	cases := []struct {
		name  string
		token string
		error string
	}{
		{"missing", "", "invalid_token"},
		{"invalid", "not-a-jwt", "invalid_token"},
		{"other-owner", makeAccessToken(t, key, func() map[string]any {
			claims := defaultClaims()
			claims["sub"] = "another-user"
			return claims
		}()), "invalid_token"},
		{"scope", makeAccessToken(t, key, func() map[string]any {
			claims := defaultClaims()
			claims["scope"] = "profile:read"
			return claims
		}()), "insufficient_scope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, result := oauthRequest(t, server, http.MethodPost, "/mcp", tc.token, call, nil)
			if res.StatusCode != 200 || result["id"] != float64(9) {
				t.Fatalf("tool auth status=%d result=%v", res.StatusCode, result)
			}
			payload := result["result"].(map[string]any)
			if payload["isError"] != true {
				t.Fatalf("missing isError: %v", payload)
			}
			meta := payload["_meta"].(map[string]any)
			challenge := meta["mcp/www_authenticate"].([]any)[0].(string)
			if !strings.Contains(challenge, `resource_metadata="https://signalspace.example/.well-known/oauth-protected-resource"`) ||
				!strings.Contains(challenge, `error="`+tc.error+`"`) || !strings.Contains(challenge, `error_description="`) {
				t.Fatalf("invalid tool-level challenge: %s", challenge)
			}
		})
	}

	// Mensagens desconhecidas, inválidas e grandes não recebem resultados de ferramentas.
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"other"}}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"connection_diagnostic"}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"connection_diagnostic"}}` + `garbage`,
		strings.Repeat("A", maxBodyBytes+1),
	} {
		res, _ := oauthRequest(t, server, http.MethodPost, "/mcp", "", body, nil)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthorized malformed request returned status %d", res.StatusCode)
		}
	}
}
