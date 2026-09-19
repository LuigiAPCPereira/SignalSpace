package mcp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func startPreflightIssuer(t *testing.T, customize func(map[string]any), status int) (string, string, *http.Client, *httptest.Server) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			if status != 0 {
				w.WriteHeader(status)
				return
			}
			fallthrough
		case "/.well-known/openid-configuration":
			metadata := map[string]any{
				"issuer":                                issuer,
				"authorization_endpoint":                issuer + "authorize",
				"token_endpoint":                        issuer + "token",
				"jwks_uri":                              issuer + "jwks",
				"response_types_supported":              []string{"code"},
				"grant_types_supported":                 []string{"authorization_code"},
				"code_challenge_methods_supported":      []string{"S256"},
				"client_id_metadata_document_supported": true,
				"token_endpoint_auth_methods_supported": []string{"none"},
			}
			if customize != nil {
				customize(metadata)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(metadata)
		case "/jwks":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
				"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "preflight",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	issuer = srv.URL + "/"
	t.Cleanup(srv.Close)
	return issuer, issuer + "jwks", srv.Client(), srv
}

func TestOAuthPreflightValidAndOIDCFallback(t *testing.T) {
	for _, code := range []int{0, http.StatusNotFound} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			issuer, jwks, client, _ := startPreflightIssuer(t, nil, code)
			report, err := CheckOAuthProvider(context.Background(), issuer, jwks, client)
			if err != nil {
				t.Fatal(err)
			}
			if report.Registration != "cimd" || report.JWKSURL != jwks || !strings.Contains(report.MetadataURL, ".well-known/") {
				t.Fatalf("unexpected report: %+v", report)
			}
		})
	}
}

func TestOAuthPreflightRejectsBrokenMetadata(t *testing.T) {
	cases := map[string]func(map[string]any){
		"issuer":        func(m map[string]any) { m["issuer"] = "https://other.example/" },
		"pkce":          func(m map[string]any) { m["code_challenge_methods_supported"] = []string{"plain"} },
		"no_code":       func(m map[string]any) { m["response_types_supported"] = []string{"token"} },
		"no_grant":      func(m map[string]any) { m["grant_types_supported"] = []string{"client_credentials"} },
		"jwks_mismatch": func(m map[string]any) { m["jwks_uri"] = "https://other.example/jwks" },
		"http_token":    func(m map[string]any) { m["token_endpoint"] = "http://example.com/token" },
		"bad_cimd": func(m map[string]any) {
			m["token_endpoint_auth_methods_supported"] = []string{"client_secret_basic"}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			issuer, jwks, client, _ := startPreflightIssuer(t, change, 0)
			if _, err := CheckOAuthProvider(context.Background(), issuer, jwks, client); err == nil {
				t.Fatal("accepted incompatible authorization server")
			}
		})
	}
}

func TestOAuthPreflightNoRedirectOrUntrustedURL(t *testing.T) {
	issuer, jwks, client, _ := startPreflightIssuer(t, nil, http.StatusFound)
	if _, err := CheckOAuthProvider(context.Background(), issuer, jwks, client); err == nil {
		t.Fatal("accepted redirect from metadata endpoint")
	}
	if _, err := CheckOAuthProvider(context.Background(), "http://example.com/", jwks, client); err == nil {
		t.Fatal("accepted insecure issuer")
	}
	if _, err := CheckOAuthProvider(context.Background(), issuer, "https://user:password@example.com/jwks", client); err == nil {
		t.Fatal("accepted credential-bearing JWKS URL")
	}
}

func TestOAuthPreflightRegistrationFallback(t *testing.T) {
	issuer, jwks, client, _ := startPreflightIssuer(t, func(m map[string]any) {
		m["client_id_metadata_document_supported"] = false
		m["registration_endpoint"] = "https://example.com/register"
	}, 0)
	report, err := CheckOAuthProvider(context.Background(), issuer, jwks, client)
	if err != nil || report.Registration != "dcr" {
		t.Fatalf("DCR discovery: report=%+v err=%v", report, err)
	}
}

func TestOAuthPreflightMetadataBoundariesAndMissingRegistration(t *testing.T) {
	for name, change := range map[string]func(map[string]any){
		"oversized":              func(m map[string]any) { m["padding"] = strings.Repeat("a", maxMetadataBytes) },
		"missing_token_endpoint": func(m map[string]any) { delete(m, "token_endpoint") },
		"invalid_registration_endpoint": func(m map[string]any) {
			m["client_id_metadata_document_supported"] = false
			m["registration_endpoint"] = "http://insecure.example/register"
		},
	} {
		t.Run(name, func(t *testing.T) {
			issuer, jwks, client, _ := startPreflightIssuer(t, change, 0)
			if _, err := CheckOAuthProvider(context.Background(), issuer, jwks, client); err == nil {
				t.Fatal("accepted invalid metadata")
			}
		})
	}
	issuer, jwks, client, _ := startPreflightIssuer(t, func(m map[string]any) {
		m["client_id_metadata_document_supported"] = false
	}, 0)
	report, err := CheckOAuthProvider(context.Background(), issuer, jwks, client)
	if err != nil || report.Registration != "pre_registered_client_required" {
		t.Fatalf("missing client registration method not disclosed: %+v %v", report, err)
	}
}

func TestOAuthPreflightRejectsBrokenJWKS(t *testing.T) {
	issuer, jwks, client, _ := startPreflightIssuer(t, nil, 0)
	original := client.Transport
	client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/jwks" {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
		return original.RoundTrip(req)
	})
	if _, err := CheckOAuthProvider(context.Background(), issuer, jwks, client); err == nil {
		t.Fatal("accepted unavailable JWKS")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
