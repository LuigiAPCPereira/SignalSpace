package mcp

import (
	"context"
	"crypto"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxJWKSBytes = 64 * 1024
const maxTokenBytes = 16 * 1024
const keyCacheTTL = 5 * time.Minute

var errInvalidToken = errors.New("invalid OAuth access token")

type JWKSVerifier struct {
	url     string
	client  *http.Client
	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	expires time.Time
}

// NewJWKSVerifier confia somente na URL HTTPS configurada pelo proprietário.
// O servidor não segue redirecionamentos e falha fechado se as chaves expirarem.
func NewJWKSVerifier(rawURL string) (*JWKSVerifier, error) {
	parsed, err := parseSecureURL(rawURL)
	if err != nil || parsed.String() != rawURL {
		return nil, errors.New("JWKS URL must be an absolute HTTPS URL")
	}
	return &JWKSVerifier{
		url: rawURL,
		client: &http.Client{
			Timeout: 3 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (v *JWKSVerifier) Verify(ctx context.Context, token, issuer, audience, scope string) error {
	if len(token) == 0 || len(token) > maxTokenBytes {
		return errInvalidToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errInvalidToken
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errInvalidToken
	}
	var header struct {
		Algorithm string   `json:"alg"`
		KeyID     string   `json:"kid"`
		Critical  []string `json:"crit"`
	}
	if len(headerBytes) > 4096 || json.Unmarshal(headerBytes, &header) != nil || header.Algorithm != "RS256" || header.KeyID == "" || len(header.Critical) != 0 {
		return errInvalidToken
	}
	keys, err := v.publicKeys(ctx)
	if err != nil {
		return errInvalidToken
	}
	key := keys[header.KeyID]
	if key == nil {
		return errInvalidToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return errInvalidToken
	}
	signed := []byte(parts[0] + "." + parts[1])
	digest := crypto.SHA256.New()
	_, _ = digest.Write(signed)
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest.Sum(nil), signature) != nil {
		return errInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(payload) > maxTokenBytes {
		return errInvalidToken
	}
	var claims struct {
		Issuer    string          `json:"iss"`
		Subject   string          `json:"sub"`
		Audience  json.RawMessage `json:"aud"`
		Expires   int64           `json:"exp"`
		NotBefore *int64          `json:"nbf"`
		Scope     string          `json:"scope"`
		Scopes    []string        `json:"scp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Issuer != issuer || claims.Subject == "" || !hasAudience(claims.Audience, audience) {
		return errInvalidToken
	}
	now := time.Now().Unix()
	if claims.Expires <= now || (claims.NotBefore != nil && *claims.NotBefore > now) {
		return errInvalidToken
	}
	for _, allowed := range append(strings.Fields(claims.Scope), claims.Scopes...) {
		if allowed == scope {
			return nil
		}
	}
	return ErrInsufficientScope
}

func hasAudience(raw json.RawMessage, expected string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == expected
	}
	var multiple []string
	if json.Unmarshal(raw, &multiple) != nil {
		return false
	}
	for _, actual := range multiple {
		if actual == expected {
			return true
		}
	}
	return false
}

func (v *JWKSVerifier) publicKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.keys) > 0 && time.Now().Before(v.expires) {
		return v.keys, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return nil, err
	}
	res, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.ContentLength > maxJWKSBytes {
		return nil, errors.New("JWKS unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxJWKSBytes+1))
	if err != nil || len(data) > maxJWKSBytes {
		return nil, errors.New("JWKS too large or unavailable")
	}
	var document struct {
		Keys []struct {
			Type       string   `json:"kty"`
			Algorithm  string   `json:"alg"`
			Usage      string   `json:"use"`
			Operations []string `json:"key_ops"`
			ID         string   `json:"kid"`
			Modulus    string   `json:"n"`
			Exponent   string   `json:"e"`
		} `json:"keys"`
	}
	if json.Unmarshal(data, &document) != nil || len(document.Keys) == 0 || len(document.Keys) > 32 {
		return nil, errors.New("invalid JWKS")
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, item := range document.Keys {
		if item.Type != "RSA" || (item.Algorithm != "" && item.Algorithm != "RS256") ||
			(item.Usage != "" && item.Usage != "sig") || item.ID == "" || len(item.ID) > 128 {
			continue
		}
		if len(item.Operations) != 0 {
			allowed := false
			for _, op := range item.Operations {
				allowed = allowed || op == "verify"
			}
			if !allowed {
				continue
			}
		}
		modulus, err := base64.RawURLEncoding.DecodeString(item.Modulus)
		if err != nil {
			return nil, err
		}
		exponent, err := base64.RawURLEncoding.DecodeString(item.Exponent)
		if err != nil || len(exponent) == 0 || len(exponent) > 4 {
			return nil, errors.New("invalid RSA exponent")
		}
		number := new(big.Int).SetBytes(exponent)
		keyN := new(big.Int).SetBytes(modulus)
		if !number.IsInt64() || number.Int64() < 3 || number.Int64() > 2147483647 || number.Int64()%2 == 0 || keyN.BitLen() < 2048 || keyN.Bit(0) == 0 {
			return nil, errors.New("weak or invalid RSA key")
		}
		if _, duplicate := keys[item.ID]; duplicate {
			return nil, fmt.Errorf("duplicate JWKS key ID")
		}
		keys[item.ID] = &rsa.PublicKey{N: keyN, E: int(number.Int64())}
	}
	if len(keys) == 0 {
		return nil, errors.New("no supported JWKS keys")
	}
	v.keys = keys
	v.expires = time.Now().Add(keyCacheTTL)
	return keys, nil
}
