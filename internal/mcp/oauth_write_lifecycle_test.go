package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

const oauthWriteIssuer = "https://signalspace.example"

type oauthWriteComposition struct {
	authorization *auth.Server
	grants        *workspace.Grants
	verifier      *JWKSVerifier
	server        *httptest.Server
	requests      chan auth.RequestInfo
}

func newOAuthWriteComposition(t *testing.T, stateDir string) *oauthWriteComposition {
	t.Helper()
	requests := make(chan auth.RequestInfo, 4)
	var grants *workspace.Grants
	authorization, err := auth.New(auth.Config{
		ResourceURL: testResource,
		Issuer:      oauthWriteIssuer,
		Scope:       diagnosticScope,
		WriteScope:  workspaceWriteScope,
		CanIssueWrite: func(clientID string) bool {
			return grants != nil && grants.AllowsClientScope(clientID, workspace.ScopeWrite)
		},
		StateDir: stateDir,
		OnRequest: func(info auth.RequestInfo) {
			requests <- info
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	grants, err = workspace.NewGrants(authorization.OwnerSubject())
	if err != nil {
		_ = authorization.Close()
		t.Fatal(err)
	}
	verifier, err := NewStaticJWTVerifier(authorization.PublicKey(), authorization.KeyID())
	if err != nil {
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatal(err)
	}
	protected, err := NewOAuthHandler(OAuthConfig{
		ResourceURL:     testResource,
		Issuer:          oauthWriteIssuer,
		OwnerSubject:    authorization.OwnerSubject(),
		workspaceWriter: grants,
	}, verifier)
	if err != nil {
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", protected)
	mux.Handle(metadataPath, protected)
	mux.Handle(metadataPath+"/mcp", protected)
	mux.Handle("/", authorization.Handler())
	composition := &oauthWriteComposition{
		authorization: authorization,
		grants:        grants,
		verifier:      verifier,
		server:        httptest.NewServer(mux),
		requests:      requests,
	}
	t.Cleanup(func() {
		_ = composition.Close()
	})
	return composition
}

func (c *oauthWriteComposition) Close() error {
	if c.server != nil {
		c.server.Close()
	}
	if err := c.grants.Close(); err != nil {
		return err
	}
	return c.authorization.Close()
}

func issueOAuthWriteToken(t *testing.T, c *oauthWriteComposition, root string) (string, string, string, url.Values) {
	t.Helper()
	registerBody := `{"client_name":"OAuth write lifecycle harness","redirect_uris":["` + oauthWriteCallback + `"],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`
	registered := oauthWriteHTTP(t, c.server, http.MethodPost, "/register", "application/json", registerBody, "", "")
	if registered.status != http.StatusCreated {
		t.Fatalf("client registration failed: %d %s", registered.status, registered.body)
	}
	registeredJSON := oauthWriteJSON(t, registered)
	clientID, ok := registeredJSON["client_id"].(string)
	if !ok || clientID == "" {
		t.Fatalf("registration did not return client_id: %v", registeredJSON)
	}
	sessionID, err := c.grants.GrantWithScopes(root, clientID, workspace.ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}

	verifierValue := strings.Repeat("v", 43)
	challengeDigest := sha256.Sum256([]byte(verifierValue))
	state := strings.Repeat("s", 16)
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {oauthWriteCallback},
		"scope":                 {diagnosticScope + " " + workspaceWriteScope},
		"resource":              {testResource},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challengeDigest[:])},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	authorizeResponse := oauthWriteHTTP(t, c.server, http.MethodGet, "/authorize?"+query.Encode(), "", "", "", "")
	if authorizeResponse.status != http.StatusOK {
		t.Fatalf("authorize failed: %d %s", authorizeResponse.status, authorizeResponse.body)
	}
	csrfMatch := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindSubmatch(authorizeResponse.body)
	if len(csrfMatch) != 2 {
		t.Fatal("authorize response did not contain CSRF form value")
	}
	cookies := (&http.Response{Header: authorizeResponse.header}).Cookies()
	if len(cookies) != 1 || cookies[0].Name != "signalspace_auth" {
		t.Fatalf("authorize response did not contain the local session cookie: %v", cookies)
	}
	var request auth.RequestInfo
	select {
	case request = <-c.requests:
	case <-time.After(3 * time.Second):
		t.Fatal("OAuth request was not delivered to the local decision queue")
	}
	if request.ClientID != clientID || request.Scope != diagnosticScope+" "+workspaceWriteScope {
		t.Fatalf("OAuth request changed before consent: %+v", request)
	}
	if err := c.authorization.DecideTerminal(request.ID, true); err != nil {
		t.Fatal(err)
	}

	completeForm := url.Values{"request": {request.ID}, "csrf": {string(csrfMatch[1])}}
	completeResponse := oauthWriteHTTP(t, c.server, http.MethodPost, "/authorize/complete", "application/x-www-form-urlencoded", completeForm.Encode(), "", cookies[0].Name+"="+cookies[0].Value)
	if completeResponse.status != http.StatusSeeOther {
		t.Fatalf("authorize completion failed: %d %s", completeResponse.status, completeResponse.body)
	}
	redirect, err := url.Parse(completeResponse.header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	code := redirect.Query().Get("code")
	if code == "" || redirect.Query().Get("state") != state || redirect.Query().Get("iss") != oauthWriteIssuer {
		t.Fatalf("OAuth redirect claims are incomplete: %s", redirect)
	}
	tokenForm := url.Values{
		"grant_type":    {"authorization_code"},
		"resource":      {testResource},
		"code":          {code},
		"code_verifier": {verifierValue},
		"client_id":     {clientID},
		"redirect_uri":  {oauthWriteCallback},
	}
	tokenResponse := oauthWriteHTTP(t, c.server, http.MethodPost, "/token", "application/x-www-form-urlencoded", tokenForm.Encode(), "", "")
	if tokenResponse.status != http.StatusOK {
		t.Fatalf("token exchange failed: %d %s", tokenResponse.status, tokenResponse.body)
	}
	issued := oauthWriteJSON(t, tokenResponse)
	accessToken, ok := issued["access_token"].(string)
	if !ok || accessToken == "" || issued["scope"] != diagnosticScope+" "+workspaceWriteScope {
		t.Fatalf("token did not preserve exact approved scope: %v", issued)
	}
	identity, err := c.verifier.VerifyIdentity(context.Background(), accessToken, oauthWriteIssuer, testResource, workspaceWriteScope, c.authorization.OwnerSubject())
	if err != nil || identity.ClientID != clientID {
		t.Fatalf("real verifier rejected issued write identity: %+v %v", identity, err)
	}
	return clientID, sessionID, accessToken, tokenForm
}

func TestOAuthWriteLocalCompositionLifecycleDropsPriorGrant(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "editable.txt")
	if err := os.WriteFile(file, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(t.TempDir(), "identity")

	first := newOAuthWriteComposition(t, stateDir)
	clientID, oldSessionID, accessToken, _ := issueOAuthWriteToken(t, first, root)
	initial := oauthWriteHTTP(t, first.server, http.MethodPost, "/mcp", "application/json", oauthWriteCall(oldSessionID, "editable.txt", "before", "after-initial"), accessToken, "")
	initialJSON := oauthWriteJSON(t, initial)
	if initial.status != http.StatusOK || initialJSON["result"].(map[string]any)["isError"] != false {
		t.Fatalf("write did not work before composition close: %d %s", initial.status, initial.body)
	}
	if content, err := os.ReadFile(file); err != nil || string(content) != "after-initial" {
		t.Fatalf("initial write did not change the disposable workspace: %q %v", content, err)
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.grants.ReplaceText(first.authorization.OwnerSubject(), clientID, oldSessionID, "editable.txt", "after-initial", "closed-resource"); !errors.Is(err, workspace.ErrClosed) {
		t.Fatalf("closed grant accepted a write: %v", err)
	}

	second := newOAuthWriteComposition(t, stateDir)
	identity, err := second.verifier.VerifyIdentity(context.Background(), accessToken, oauthWriteIssuer, testResource, workspaceWriteScope, second.authorization.OwnerSubject())
	if err != nil || identity.ClientID != clientID {
		t.Fatalf("persisted identity did not keep the old JWT cryptographically valid: %+v %v", identity, err)
	}
	withoutGrant := oauthWriteHTTP(t, second.server, http.MethodPost, "/mcp", "application/json", oauthWriteCall(oldSessionID, "editable.txt", "after-initial", "implicit-grant"), accessToken, "")
	withoutGrantJSON := oauthWriteJSON(t, withoutGrant)
	if withoutGrant.status != http.StatusOK || withoutGrantJSON["result"].(map[string]any)["isError"] != true {
		t.Fatalf("old token/session edited without an active grant: %d %s", withoutGrant.status, withoutGrant.body)
	}
	if content, err := os.ReadFile(file); err != nil || string(content) != "after-initial" {
		t.Fatalf("write without a recreated grant changed the workspace: %q %v", content, err)
	}

	newSessionID, err := second.grants.GrantWithScopes(root, clientID, workspace.ScopeWrite)
	if err != nil {
		t.Fatal(err)
	}
	withNewGrant := oauthWriteHTTP(t, second.server, http.MethodPost, "/mcp", "application/json", oauthWriteCall(newSessionID, "editable.txt", "after-initial", "after-recreated"), accessToken, "")
	withNewGrantJSON := oauthWriteJSON(t, withNewGrant)
	if withNewGrant.status != http.StatusOK || withNewGrantJSON["result"].(map[string]any)["isError"] != false {
		t.Fatalf("explicit recreated grant did not authorize the compatible old token: %d %s", withNewGrant.status, withNewGrant.body)
	}
	if content, err := os.ReadFile(file); err != nil || string(content) != "after-recreated" {
		t.Fatalf("recreated grant write did not change the disposable workspace: %q %v", content, err)
	}
	oldSessionAfterRegrant := oauthWriteHTTP(t, second.server, http.MethodPost, "/mcp", "application/json", oauthWriteCall(oldSessionID, "editable.txt", "after-recreated", "stale-session"), accessToken, "")
	oldSessionAfterRegrantJSON := oauthWriteJSON(t, oldSessionAfterRegrant)
	if oldSessionAfterRegrant.status != http.StatusOK || oldSessionAfterRegrantJSON["result"].(map[string]any)["isError"] != true {
		t.Fatalf("old session remained valid after explicit new grant: %d %s", oldSessionAfterRegrant.status, oldSessionAfterRegrant.body)
	}
	if content, err := os.ReadFile(file); err != nil || string(content) != "after-recreated" {
		t.Fatalf("stale session rejection changed the workspace: %q %v", content, err)
	}
}
