package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestVerifiedIdentityRejectsUnboundAndTamperedTokens(t *testing.T) {
	key, verifier, _, _ := setupOAuth(t)
	ctx := context.Background()
	claims := defaultClaims()
	clientID := strings.Repeat("A", 32)
	claims["client_id"] = clientID
	token := makeAccessToken(t, key, claims)

	identity, err := verifier.VerifyIdentity(ctx, token, testIssuer, testResource, diagnosticScope, "owner-test")
	if err != nil || identity.OwnerSubject != "owner-test" || identity.ClientID != clientID {
		t.Fatalf("verified identity mismatch: %+v, %v", identity, err)
	}
	if _, err := verifier.VerifyIdentity(ctx, token, testIssuer, testResource, "signalspace:workspace.read", "owner-test"); !errors.Is(err, ErrInsufficientScope) {
		t.Fatalf("diagnostic token authorized read: %v", err)
	}
	if _, err := verifier.VerifyIdentity(ctx, token+"tampered", testIssuer, testResource, diagnosticScope, "owner-test"); !errors.Is(err, errInvalidToken) {
		t.Fatalf("tampered signature accepted: %v", err)
	}

	for name, client := range map[string]any{
		"absent": nil, "empty": "", "short": "different-client", "space": strings.Repeat("A", 31) + " ",
		"newline": strings.Repeat("A", 31) + "\n", "non-string": 123,
	} {
		t.Run(name, func(t *testing.T) {
			altered := defaultClaims()
			if client != nil {
				altered["client_id"] = client
			}
			signed := makeAccessToken(t, key, altered)
			// Um JWT legado ainda serve ao diagnóstico, mas não identifica
			// um cliente para acesso a arquivos.
			if err := verifier.Verify(ctx, signed, testIssuer, testResource, diagnosticScope, "owner-test"); err != nil {
				t.Fatalf("diagnostic regression: %v", err)
			}
			if got, err := verifier.VerifyIdentity(ctx, signed, testIssuer, testResource, diagnosticScope, "owner-test"); !errors.Is(err, errInvalidToken) || got != (VerifiedIdentity{}) {
				t.Fatalf("unbound identity accepted: %+v, %v", got, err)
			}
		})
	}

	for name, change := range map[string]func(map[string]any){
		"wrong_owner":    func(c map[string]any) { c["sub"] = "other-owner" },
		"wrong_audience": func(c map[string]any) { c["aud"] = "https://other.example/mcp" },
		"wrong_issuer":   func(c map[string]any) { c["iss"] = "https://other.example" },
		"expired":        func(c map[string]any) { c["exp"] = time.Now().Add(-time.Second).Unix() },
	} {
		t.Run(name, func(t *testing.T) {
			altered := defaultClaims()
			altered["client_id"] = clientID
			change(altered)
			if _, err := verifier.VerifyIdentity(ctx, makeAccessToken(t, key, altered), testIssuer, testResource, diagnosticScope, "owner-test"); !errors.Is(err, errInvalidToken) {
				t.Fatalf("invalid claims accepted: %v", err)
			}
		})
	}
}

func TestVerifiedIdentityRequiresExplicitReadScope(t *testing.T) {
	key, verifier, _, _ := setupOAuth(t)
	claims := defaultClaims()
	claims["client_id"] = strings.Repeat("B", 32)
	claims["scope"] = "signalspace:workspace.read"
	// Esta concessão é sintética e testa somente o verificador.
	// O servidor de autorização real ainda emite apenas diagnóstico.
	identity, err := verifier.VerifyIdentity(context.Background(), makeAccessToken(t, key, claims), testIssuer, testResource, "signalspace:workspace.read", "owner-test")
	if err != nil || identity.ClientID != strings.Repeat("B", 32) {
		t.Fatalf("signed read scope rejected: %+v, %v", identity, err)
	}
}
