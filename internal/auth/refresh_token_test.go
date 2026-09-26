package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

func refreshConfig(dir string, migrate bool) Config {
	return Config{
		ResourceURL:         resourceURL,
		Issuer:              "https://signalspace.example",
		Scope:               scope,
		CompositionScope:    programmingScope,
		EnableRefreshTokens: true,
		MigrateState:        migrate,
		StateDir:            dir,
	}
}

func registerRefreshClient(t *testing.T, h http.Handler, grants ...string) string {
	t.Helper()
	if len(grants) == 0 {
		grants = []string{"authorization_code", "refresh_token"}
	}
	body := `{"client_name":"ChatGPT","redirect_uris":["` + callback + `"],"grant_types":["` + strings.Join(grants, `","`) + `"],"response_types":["code"],"token_endpoint_auth_method":"none"}`
	response := invoke(h, http.MethodPost, "/register", body, "application/json", nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("refresh client registration failed: %d %s", response.Code, response.Body.String())
	}
	var result struct {
		ClientID   string   `json:"client_id"`
		GrantTypes []string `json:"grant_types"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.ClientID == "" {
		t.Fatalf("invalid refresh registration response: %s", response.Body.String())
	}
	if !reflect.DeepEqual(result.GrantTypes, grants) {
		t.Fatalf("registration grant negotiation: got=%v want=%v", result.GrantTypes, grants)
	}
	return result.ClientID
}

func requestScopeConsent(t *testing.T, h http.Handler, clientID, requestedScope string) (*http.Cookie, string, string) {
	t.Helper()
	digest := sha256Bytes(testVerifier)
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {callback},
		"scope":                 {requestedScope},
		"resource":              {resourceURL},
		"code_challenge":        {digest},
		"code_challenge_method": {"S256"},
		"state":                 {"state-refresh-token-test"},
	}
	response := invoke(h, http.MethodGet, "/authorize?"+query.Encode(), "", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("scope authorization request failed: %d %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("scope authorization did not issue a session cookie")
	}
	csrf := regexpExtract(response.Body.String(), `name="csrf" value="([^"]+)"`)
	requestID := regexpExtract(response.Body.String(), `data-request-id="([^"]+)"`)
	if csrf == "" || requestID == "" {
		t.Fatal("scope authorization omitted CSRF or request ID")
	}
	return cookies[0], csrf, requestID
}

func sha256Bytes(value string) string {
	digest := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func regexpExtract(value, expression string) string {
	match := regexp.MustCompile(expression).FindStringSubmatch(value)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func issueRefreshToken(t *testing.T, server *Server, h http.Handler, clientID, requestedScope string) oauthTokenResponse {
	t.Helper()
	cookie, csrf, requestID := requestScopeConsent(t, h, clientID, requestedScope)
	if err := server.Approve(requestID, true); err != nil {
		t.Fatal(err)
	}
	redirect := complete(h, requestID, csrf, cookie)
	if redirect.Code != http.StatusSeeOther {
		t.Fatalf("authorization completion failed: %d %s", redirect.Code, redirect.Body.String())
	}
	callbackURL, err := url.Parse(redirect.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"redirect_uri":  {callback},
		"code":          {callbackURL.Query().Get("code")},
		"code_verifier": {testVerifier},
		"resource":      {resourceURL},
	}
	response := invoke(h, http.MethodPost, "/token", form.Encode(), "application/x-www-form-urlencoded", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("authorization-code exchange failed: %d %s", response.Code, response.Body.String())
	}
	var token oauthTokenResponse
	if err := json.Unmarshal(response.Body.Bytes(), &token); err != nil || token.RefreshToken == "" || token.AccessToken == "" {
		t.Fatalf("authorization-code exchange omitted token family: %s", response.Body.String())
	}
	return token
}

func refreshRequest(h http.Handler, clientID, refreshToken, resource, requestedScope string) *httptest.ResponseRecorder {
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}, "client_id": {clientID}, "resource": {resource}}
	if requestedScope != "" {
		form.Set("scope", requestedScope)
	}
	return invoke(h, http.MethodPost, "/token", form.Encode(), "application/x-www-form-urlencoded", nil)
}

func TestRefreshLifecycleMetadataDCRAndRotation(t *testing.T) {
	server, err := New(refreshConfig(filepath.Join(t.TempDir(), "state"), false))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	h := server.Handler()
	metadata := invoke(h, http.MethodGet, "/.well-known/oauth-authorization-server", "", "", nil)
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), `"refresh_token"`) || !strings.Contains(metadata.Body.String(), programmingScope) {
		t.Fatalf("v2 metadata incomplete: %d %s", metadata.Code, metadata.Body.String())
	}
	clientID := registerRefreshClient(t, h)
	token := issueRefreshToken(t, server, h, clientID, programmingScope)
	if token.TokenType != "Bearer" || token.Scope != programmingScope || token.ExpiresIn != int(v2AccessTokenTTL.Seconds()) {
		t.Fatalf("unexpected issued token metadata: %+v", token)
	}
	state, err := os.ReadFile(filepath.Join(server.store.dir, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), token.RefreshToken) {
		t.Fatal("refresh token secret was persisted in cleartext")
	}
	if !strings.Contains(string(state), hashRefreshToken(token.RefreshToken)) {
		t.Fatal("refresh-token hash was not persisted")
	}

	rotated := refreshRequest(h, clientID, token.RefreshToken, resourceURL, "")
	if rotated.Code != http.StatusOK {
		t.Fatalf("valid refresh failed: %d %s", rotated.Code, rotated.Body.String())
	}
	var next oauthTokenResponse
	if err := json.Unmarshal(rotated.Body.Bytes(), &next); err != nil || next.RefreshToken == "" || next.RefreshToken == token.RefreshToken || next.Scope != programmingScope {
		t.Fatalf("refresh did not rotate token: %s", rotated.Body.String())
	}
	oldReuse := refreshRequest(h, clientID, token.RefreshToken, resourceURL, "")
	if oldReuse.Code != http.StatusBadRequest {
		t.Fatalf("rotated refresh token was reusable: %d %s", oldReuse.Code, oldReuse.Body.String())
	}
	if afterReuse := refreshRequest(h, clientID, next.RefreshToken, resourceURL, ""); afterReuse.Code != http.StatusBadRequest {
		t.Fatalf("refresh-token family was not revoked after reuse: %d %s", afterReuse.Code, afterReuse.Body.String())
	}
}

func TestRefreshLifecycleRejectsExpansionAndBindingMismatch(t *testing.T) {
	server, err := New(refreshConfig(filepath.Join(t.TempDir(), "state"), false))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	h := server.Handler()
	clientID := registerRefreshClient(t, h)
	otherClient := registerRefreshClient(t, h)
	token := issueRefreshToken(t, server, h, clientID, programmingScope)
	if response := refreshRequest(h, clientID, token.RefreshToken, resourceURL, scope+" "+programmingScope); response.Code != http.StatusBadRequest {
		t.Fatalf("scope expansion accepted: %d %s", response.Code, response.Body.String())
	}
	if response := refreshRequest(h, otherClient, token.RefreshToken, resourceURL, ""); response.Code != http.StatusBadRequest {
		t.Fatalf("client mismatch accepted: %d %s", response.Code, response.Body.String())
	}
	if response := refreshRequest(h, clientID, token.RefreshToken, resourceURL+"/other", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("resource mismatch accepted: %d %s", response.Code, response.Body.String())
	}
	server.mu.Lock()
	for id, family := range server.families {
		family.ExpiresAt = time.Now().Add(-time.Second)
		server.families[id] = family
	}
	server.mu.Unlock()
	if response := refreshRequest(h, clientID, token.RefreshToken, resourceURL, ""); response.Code != http.StatusBadRequest {
		t.Fatalf("expired family accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestRefreshLifecycleRestartAndExplicitMigration(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	legacy, err := New(durableConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	if _, err := New(refreshConfig(dir, false)); err == nil || !strings.Contains(err.Error(), "explicit migration") {
		t.Fatalf("v1 state opened without explicit migration: %v", err)
	}
	server, err := New(refreshConfig(dir, true))
	if err != nil {
		t.Fatal(err)
	}
	clientID := registerRefreshClient(t, server.Handler())
	token := issueRefreshToken(t, server, server.Handler(), clientID, programmingScope)
	server.mu.Lock()
	var familyID string
	for id, family := range server.families {
		if family.ClientID == clientID {
			familyID = id
			break
		}
	}
	server.mu.Unlock()
	if familyID == "" {
		t.Fatal("refresh token family was not created")
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(refreshConfig(dir, false))
	if err != nil {
		t.Fatal(err)
	}
	response := refreshRequest(restarted.Handler(), clientID, token.RefreshToken, resourceURL, "")
	if response.Code != http.StatusOK {
		t.Fatalf("valid family did not survive restart: %d %s", response.Code, response.Body.String())
	}
	var rotated oauthTokenResponse
	if err := json.Unmarshal(response.Body.Bytes(), &rotated); err != nil || rotated.RefreshToken == "" {
		t.Fatalf("restart response omitted rotated refresh token: %s", response.Body.String())
	}
	if err := restarted.RevokeTokenFamily(familyID); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}
	revoked, err := New(refreshConfig(dir, false))
	if err != nil {
		t.Fatal(err)
	}
	defer revoked.Close()
	if response := refreshRequest(revoked.Handler(), clientID, rotated.RefreshToken, resourceURL, ""); response.Code != http.StatusBadRequest {
		t.Fatalf("revoked family survived restart: %d %s", response.Code, response.Body.String())
	}
}

func TestLegacyRuntimeDoesNotAnnounceV2Lifecycle(t *testing.T) {
	server, err := New(durableConfig(filepath.Join(t.TempDir(), "state")))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	metadata := invoke(server.Handler(), http.MethodGet, "/.well-known/oauth-authorization-server", "", "", nil)
	if metadata.Code != http.StatusOK || strings.Contains(metadata.Body.String(), `"refresh_token"`) || strings.Contains(metadata.Body.String(), programmingScope) {
		t.Fatalf("legacy metadata announced v2 lifecycle: %d %s", metadata.Code, metadata.Body.String())
	}
}

func TestRefreshLifecycleConcurrentSingleSuccessor(t *testing.T) {
	server, err := New(refreshConfig(filepath.Join(t.TempDir(), "state"), false))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	h := server.Handler()
	clientID := registerRefreshClient(t, h)
	token := issueRefreshToken(t, server, h, clientID, programmingScope)
	const workers = 12
	var group sync.WaitGroup
	start := make(chan struct{})
	results := make(chan int, workers)
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results <- refreshRequest(h, clientID, token.RefreshToken, resourceURL, "").Code
		}()
	}
	close(start)
	group.Wait()
	close(results)
	success := 0
	for status := range results {
		if status == http.StatusOK {
			success++
		} else if status != http.StatusBadRequest {
			t.Fatalf("unexpected concurrent refresh status: %d", status)
		}
	}
	if success != 1 {
		t.Fatalf("concurrent refreshes produced %d successors", success)
	}
}
