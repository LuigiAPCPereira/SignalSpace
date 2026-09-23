package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
	"github.com/LuigiAPCPereira/SignalSpace/internal/programming"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

type oauthProgrammingComposition struct {
	authorization *auth.Server
	grants        *workspace.Grants
	approval      *workspace.CapabilityApproval
	verifier      *JWKSVerifier
	server        *httptest.Server
	requests      chan auth.RequestInfo
	clientID      string
	sessionID     string
	runner        *harnessTestRunner
	gitStarts     *atomic.Int32
}

func newOAuthProgrammingComposition(t *testing.T, stateDir, root string) *oauthProgrammingComposition {
	t.Helper()
	requests := make(chan auth.RequestInfo, 8)
	var grants *workspace.Grants
	authorization, err := auth.New(auth.Config{
		ResourceURL: testResource,
		Issuer:      oauthWriteIssuer,
		Scope:       diagnosticScope,
		ReadScope:   workspaceReadScope,
		CanIssueRead: func(clientID string) bool {
			return grants != nil && grants.AllowsClientScope(clientID, workspace.ScopeRead)
		},
		WriteScope: workspaceWriteScope,
		CanIssueWrite: func(clientID string) bool {
			return grants != nil && grants.AllowsClientScope(clientID, workspace.ScopeWrite)
		},
		GitScope: gitReviewScope,
		CanIssueGit: func(clientID string) bool {
			return grants != nil && grants.AllowsClientScope(clientID, workspace.ScopeGit)
		},
		TestScope: testRunScope,
		CanIssueTest: func(clientID string) bool {
			return grants != nil && grants.AllowsClientScope(clientID, workspace.ScopeTest)
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

	registrationServer := httptest.NewServer(authorization.Handler())
	registerBody := `{"client_name":"OAuth programming harness","redirect_uris":["` + oauthWriteCallback + `"],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`
	registered := oauthWriteHTTP(t, registrationServer, http.MethodPost, "/register", "application/json", registerBody, "", "")
	if registered.status != http.StatusCreated {
		registrationServer.Close()
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatalf("client registration failed: %d %s", registered.status, registered.body)
	}
	registeredJSON := oauthWriteJSON(t, registered)
	clientID, ok := registeredJSON["client_id"].(string)
	if !ok || clientID == "" {
		registrationServer.Close()
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatalf("registration did not return client_id: %v", registeredJSON)
	}
	noGrant := oauthWriteHTTP(t, registrationServer, http.MethodGet, "/authorize?"+oauthProgrammingAuthorizeQuery(clientID, diagnosticScope+" "+workspaceWriteScope).Encode(), "", "", "", "")
	if noGrant.status != http.StatusForbidden {
		registrationServer.Close()
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatalf("OAuth authorize accepted programming scope without a local grant: %d %s", noGrant.status, noGrant.body)
	}
	select {
	case request := <-requests:
		registrationServer.Close()
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatalf("OAuth created a pending approval without a local grant: %+v", request)
	default:
	}
	// O mecanismo confiável de seleção só lista clientes que já concluíram
	// OAuth. Obter o token diagnóstico não concede workspace nem capacidade de
	// programação; apenas torna o cliente elegível para a decisão local.
	diagnosticComposition := &oauthProgrammingComposition{authorization: authorization, verifier: verifier, server: registrationServer, requests: requests, clientID: clientID}
	diagnosticToken, _, _ := issueOAuthProgrammingToken(t, diagnosticComposition, diagnosticScope)
	issuedClients := authorization.IssuedClients()
	if len(issuedClients) != 1 || issuedClients[0].ID != clientID {
		registrationServer.Close()
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatal("diagnostic OAuth did not make the client eligible")
	}
	approval, err := workspace.NewCapabilityApproval(grants, func(candidate string) bool {
		for _, issued := range authorization.IssuedClients() {
			if issued.ID == candidate {
				return true
			}
		}
		return false
	})
	if err != nil {
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatal(err)
	}
	pending, err := approval.Request(root, clientID, workspace.ScopeWrite, workspace.ScopeTest, workspace.ScopeGit, workspace.ScopeRead)
	if err != nil {
		approval.Close()
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatal(err)
	}
	if grants.AllowsClientScope(clientID, workspace.ScopeRead) || grants.AllowsClientScope(clientID, workspace.ScopeWrite) || grants.AllowsClientScope(clientID, workspace.ScopeTest) || grants.AllowsClientScope(clientID, workspace.ScopeGit) {
		approval.Close()
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatal("programming request created a grant before local confirmation")
	}
	// A solicitação pendente não concede leitura nem escrita, mesmo quando as
	// portas existem numa composição experimental.
	pendingHandler, handlerErr := NewOAuthHandler(OAuthConfig{
		ResourceURL:     testResource,
		Issuer:          oauthWriteIssuer,
		OwnerSubject:    authorization.OwnerSubject(),
		WorkspaceReader: grants,
		workspaceWriter: grants,
	}, verifier)
	if handlerErr != nil {
		approval.Close()
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatal(handlerErr)
	}
	pendingServer := httptest.NewServer(pendingHandler)
	for name, call := range map[string]string{
		"read":  readCall(strings.Repeat("p", 32), "editable.txt"),
		"write": oauthWriteCall(strings.Repeat("p", 32), "editable.txt", "before\n", "pending\n"),
	} {
		response := oauthWriteHTTP(t, pendingServer, http.MethodPost, "/mcp", "application/json", call, diagnosticToken, "")
		_, isError := readResult(t, oauthWriteJSON(t, response))
		if response.status != http.StatusOK || !isError {
			pendingServer.Close()
			approval.Close()
			_ = grants.Close()
			_ = authorization.Close()
			t.Fatalf("pending approval authorized %s: %d %s", name, response.status, response.body)
		}
	}
	pendingServer.Close()
	for _, scope := range []string{workspaceReadScope, workspaceWriteScope} {
		query := oauthProgrammingAuthorizeQuery(clientID, diagnosticScope+" "+scope)
		response := oauthWriteHTTP(t, registrationServer, http.MethodGet, "/authorize?"+query.Encode(), "", "", "", "")
		if response.status != http.StatusForbidden {
			approval.Close()
			_ = grants.Close()
			_ = authorization.Close()
			t.Fatalf("pending approval allowed OAuth scope %q: %d %s", scope, response.status, response.body)
		}
	}
	select {
	case request := <-requests:
		approval.Close()
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatalf("pending approval created an OAuth consent request: %+v", request)
	default:
	}
	registrationServer.Close()
	confirmed, sessionID, err := approval.Confirm(pending.ID)
	if err != nil {
		approval.Close()
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatal(err)
	}
	if confirmed.ID != pending.ID || len(confirmed.Scopes) != 4 || confirmed.Scopes[0] != workspace.ScopeRead || confirmed.Scopes[1] != workspace.ScopeWrite || confirmed.Scopes[2] != workspace.ScopeGit || confirmed.Scopes[3] != workspace.ScopeTest {
		approval.Close()
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatalf("unexpected local programming approval: %+v", confirmed)
	}
	var baseline programming.GitSnapshot
	err = grants.WithAuthorizedGitProcessDir(authorization.OwnerSubject(), clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		var err error
		baseline, err = programming.CaptureGitSnapshot(context.Background(), directory, 8192)
		return err
	})
	if err != nil {
		_ = grants.Close()
		_ = authorization.Close()
		t.Fatal(err)
	}
	runner := &harnessTestRunner{grants: grants, timeout: 30 * time.Second, outputLimit: 8192}
	gitStarts := &atomic.Int32{}
	protected, err := NewOAuthHandler(OAuthConfig{
		ResourceURL:     testResource,
		Issuer:          oauthWriteIssuer,
		OwnerSubject:    authorization.OwnerSubject(),
		WorkspaceReader: grants,
		workspaceWriter: grants,
		gitReviewer:     &harnessGitReviewer{grants: grants, before: baseline, outputLimit: 8192, starts: gitStarts},
		testRunner:      runner,
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
	composition := &oauthProgrammingComposition{
		authorization: authorization,
		grants:        grants,
		approval:      approval,
		verifier:      verifier,
		server:        httptest.NewServer(mux),
		requests:      requests,
		clientID:      clientID,
		sessionID:     sessionID,
		runner:        runner,
		gitStarts:     gitStarts,
	}
	t.Cleanup(func() { _ = composition.Close() })
	return composition
}

func (c *oauthProgrammingComposition) Close() error {
	if c.server != nil {
		c.server.Close()
	}
	if c.approval != nil {
		c.approval.Close()
	}
	if err := c.grants.Close(); err != nil {
		return err
	}
	return c.authorization.Close()
}

func issueOAuthProgrammingToken(t *testing.T, c *oauthProgrammingComposition, requestedScope string) (string, url.Values, map[string]any) {
	t.Helper()
	verifierValue := strings.Repeat("v", 43)
	challengeDigest := sha256.Sum256([]byte(verifierValue))
	state := strings.Repeat("s", 16)
	query := oauthProgrammingAuthorizeQuery(c.clientID, requestedScope)
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challengeDigest[:]))
	query.Set("state", state)
	authorizeResponse := oauthWriteHTTP(t, c.server, http.MethodGet, "/authorize?"+query.Encode(), "", "", "", "")
	if authorizeResponse.status != http.StatusOK {
		t.Fatalf("authorize %q failed: %d %s", requestedScope, authorizeResponse.status, authorizeResponse.body)
	}
	csrfMatch := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindSubmatch(authorizeResponse.body)
	if len(csrfMatch) != 2 {
		t.Fatal("authorize response did not contain CSRF form value")
	}
	cookies := (&http.Response{Header: authorizeResponse.header}).Cookies()
	if len(cookies) != 1 || cookies[0].Name != "signalspace_auth" {
		t.Fatalf("authorize response did not contain local session cookie: %v", cookies)
	}
	var request auth.RequestInfo
	select {
	case request = <-c.requests:
	case <-time.After(3 * time.Second):
		t.Fatal("OAuth request was not delivered to local decision queue")
	}
	if request.ClientID != c.clientID || request.Scope != requestedScope {
		t.Fatalf("OAuth request changed before consent: %+v", request)
	}
	if err := c.authorization.DecideTerminal(request.ID, true); err != nil {
		t.Fatal(err)
	}
	completeForm := url.Values{"request": {request.ID}, "csrf": {string(csrfMatch[1])}}
	completeResponse := oauthWriteHTTP(t, c.server, http.MethodPost, "/authorize/complete", "application/x-www-form-urlencoded", completeForm.Encode(), "", cookies[0].Name+"="+cookies[0].Value)
	if completeResponse.status != http.StatusSeeOther {
		t.Fatalf("authorize completion failed for %q: %d %s", requestedScope, completeResponse.status, completeResponse.body)
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
		"client_id":     {c.clientID},
		"redirect_uri":  {oauthWriteCallback},
	}
	tokenResponse := oauthWriteHTTP(t, c.server, http.MethodPost, "/token", "application/x-www-form-urlencoded", tokenForm.Encode(), "", "")
	if tokenResponse.status != http.StatusOK {
		t.Fatalf("token exchange failed for %q: %d %s", requestedScope, tokenResponse.status, tokenResponse.body)
	}
	issued := oauthWriteJSON(t, tokenResponse)
	token, ok := issued["access_token"].(string)
	if !ok || token == "" || issued["scope"] != requestedScope {
		t.Fatalf("token did not preserve exact approved scope %q: %v", requestedScope, issued)
	}
	claims := oauthWriteClaims(t, token)
	if claims["iss"] != oauthWriteIssuer || claims["aud"] != testResource || claims["sub"] != c.authorization.OwnerSubject() || claims["client_id"] != c.clientID || claims["scope"] != requestedScope {
		t.Fatalf("issued OAuth claims changed or contain an unapproved scope: %v", claims)
	}
	requiredScope := diagnosticScope
	if strings.Contains(requestedScope, gitReviewScope) {
		requiredScope = gitReviewScope
	} else if strings.Contains(requestedScope, testRunScope) {
		requiredScope = testRunScope
	} else if strings.Contains(requestedScope, workspaceReadScope) {
		requiredScope = workspaceReadScope
	} else if strings.Contains(requestedScope, workspaceWriteScope) {
		requiredScope = workspaceWriteScope
	}
	if _, err := c.verifier.VerifyIdentity(context.Background(), token, oauthWriteIssuer, testResource, requiredScope, c.authorization.OwnerSubject()); err != nil {
		t.Fatalf("real static JWT verifier rejected %q token: %v", requestedScope, err)
	}
	return token, tokenForm, claims
}

func oauthProgrammingAuthorizeQuery(clientID, requestedScope string) url.Values {
	verifierValue := strings.Repeat("v", 43)
	challengeDigest := sha256.Sum256([]byte(verifierValue))
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {oauthWriteCallback},
		"scope":                 {requestedScope},
		"resource":              {testResource},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challengeDigest[:])},
		"code_challenge_method": {"S256"},
		"state":                 {strings.Repeat("s", 16)},
	}
}

func TestOAuthProgrammingVerticalFlowUsesRealIssuerVerifierAndIndependentRevocation(t *testing.T) {
	root := testRunnerFixture(t, false)
	composition := newOAuthProgrammingComposition(t, filepath.Join(t.TempDir(), "identity"), root)
	writeScope := diagnosticScope + " " + workspaceWriteScope
	readScope := diagnosticScope + " " + workspaceReadScope
	gitScope := diagnosticScope + " " + gitReviewScope
	testScope := diagnosticScope + " " + testRunScope
	writeToken, writeTokenForm, _ := issueOAuthProgrammingToken(t, composition, writeScope)
	readToken, _, _ := issueOAuthProgrammingToken(t, composition, readScope)
	gitToken, _, _ := issueOAuthProgrammingToken(t, composition, gitScope)
	testToken, _, _ := issueOAuthProgrammingToken(t, composition, testScope)

	metadataResponse := oauthWriteHTTP(t, composition.server, http.MethodGet, metadataPath, "", "", "", "")
	if metadataResponse.status != http.StatusOK {
		t.Fatalf("protected resource metadata failed: %d %s", metadataResponse.status, metadataResponse.body)
	}
	metadata := oauthWriteJSON(t, metadataResponse)
	supported := metadata["scopes_supported"].([]any)
	for _, required := range []string{diagnosticScope, workspaceReadScope, workspaceWriteScope, gitReviewScope, testRunScope} {
		found := false
		for _, item := range supported {
			if item == required {
				found = true
			}
		}
		if !found {
			t.Fatalf("real protected metadata omitted %q: %v", required, supported)
		}
	}

	for name, token := range map[string]string{"read": readToken, "write": writeToken, "git": gitToken, "test": testToken} {
		response := oauthWriteHTTP(t, composition.server, http.MethodPost, "/mcp", "application/json", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, token, "")
		if response.status != http.StatusOK {
			t.Fatalf("tools/list failed for %s token: %d %s", name, response.status, response.body)
		}
		names := toolNames(t, oauthWriteJSON(t, response))
		if name == "read" && (len(names) != 2 || names[1] != readToolName) {
			t.Fatalf("read token exposed unexpected tools: %v", names)
		}
		if name == "write" && (len(names) != 2 || names[1] != writeToolName) {
			t.Fatalf("write token exposed unexpected tools: %v", names)
		}
		if name == "git" && (len(names) != 2 || names[1] != gitReviewToolName) {
			t.Fatalf("Git token exposed unexpected tools: %v", names)
		}
		if name == "test" && (len(names) != 2 || names[1] != testRunToolName) {
			t.Fatalf("test token exposed unexpected tools: %v", names)
		}
	}

	readResponse := oauthWriteHTTP(t, composition.server, http.MethodPost, "/mcp", "application/json", readCall(composition.sessionID, "editable.txt"), readToken, "")
	readText, readError := readResult(t, oauthWriteJSON(t, readResponse))
	if readResponse.status != http.StatusOK || readError || readText != "before\n" {
		t.Fatalf("real OAuth read did not return fixture contents: %d %q %s", readResponse.status, readText, readResponse.body)
	}
	for name, token := range map[string]string{"write": writeToken, "git": gitToken, "test": testToken} {
		response := oauthWriteHTTP(t, composition.server, http.MethodPost, "/mcp", "application/json", readCall(composition.sessionID, "editable.txt"), token, "")
		text, isError := readResult(t, oauthWriteJSON(t, response))
		if response.status != http.StatusOK || !isError || strings.Contains(text, readText) {
			t.Fatalf("%s-only token read workspace contents: %d %q", name, response.status, text)
		}
	}
	wrongSessionRead := oauthWriteHTTP(t, composition.server, http.MethodPost, "/mcp", "application/json", readCall(strings.Repeat("x", len(composition.sessionID)), "editable.txt"), readToken, "")
	wrongSessionText, wrongSessionError := readResult(t, oauthWriteJSON(t, wrongSessionRead))
	if wrongSessionRead.status != http.StatusOK || !wrongSessionError || strings.Contains(wrongSessionText, readText) {
		t.Fatalf("read accepted a mismatched session: %d %q", wrongSessionRead.status, wrongSessionText)
	}
	startsBeforeDenied := composition.runner.starts.Load()
	gitStartsBeforeDenied := composition.gitStarts.Load()
	for name, call := range map[string]string{
		"write": oauthWriteCall(composition.sessionID, "editable.txt", readText, "read-only-write\n"),
		"test":  testWorkspaceCall(composition.sessionID),
		"git":   gitReviewCall(composition.sessionID),
	} {
		response := oauthWriteHTTP(t, composition.server, http.MethodPost, "/mcp", "application/json", call, readToken, "")
		payload := oauthWriteJSON(t, response)
		text, isError := readResult(t, payload)
		if response.status != http.StatusOK || !isError {
			t.Fatalf("read-only token acquired %s capability: %d %q", name, response.status, text)
		}
	}
	if composition.runner.starts.Load() != startsBeforeDenied || composition.gitStarts.Load() != gitStartsBeforeDenied || fileText(t, filepath.Join(root, "editable.txt")) != readText {
		t.Fatal("read-only token started a process or changed the workspace")
	}

	// O conteúdo lido acima é a pré-condição real da edição otimista.
	editResponse := oauthWriteHTTP(t, composition.server, http.MethodPost, "/mcp", "application/json", oauthWriteCall(composition.sessionID, "editable.txt", readText, "after\n"), writeToken, "")
	edit := oauthWriteJSON(t, editResponse)
	if editResponse.status != http.StatusOK || edit["result"].(map[string]any)["isError"] != false || fileText(t, filepath.Join(root, "editable.txt")) != "after\n" {
		t.Fatalf("real OAuth edit failed: %d %v", editResponse.status, edit)
	}

	testResponse := oauthWriteHTTP(t, composition.server, http.MethodPost, "/mcp", "application/json", testWorkspaceCall(composition.sessionID), testToken, "")
	testText, testError := readResult(t, oauthWriteJSON(t, testResponse))
	if testResponse.status != http.StatusOK || testError {
		t.Fatalf("real OAuth test execution failed: %d %s", testResponse.status, testResponse.body)
	}
	var testResult testRunResult
	if err := json.Unmarshal([]byte(testText), &testResult); err != nil {
		t.Fatal(err)
	}
	if testResult.Status != string(programming.TestPassed) || testResult.ExitCode != 0 || !testResult.Terminated || testResult.TimedOut || testResult.Canceled || testResult.OutputTruncated {
		t.Fatalf("unexpected real OAuth test result: %+v", testResult)
	}

	gitResponse := oauthWriteHTTP(t, composition.server, http.MethodPost, "/mcp", "application/json", gitReviewCall(composition.sessionID), gitToken, "")
	gitText, gitError := readResult(t, oauthWriteJSON(t, gitResponse))
	if gitResponse.status != http.StatusOK || gitError || !strings.Contains(gitText, "+after") {
		t.Fatalf("real OAuth Git review failed: %d %s", gitResponse.status, gitResponse.body)
	}

	if replay := oauthWriteHTTP(t, composition.server, http.MethodPost, "/token", "application/x-www-form-urlencoded", writeTokenForm.Encode(), "", ""); replay.status != http.StatusBadRequest {
		t.Fatalf("authorization code was reusable: %d %s", replay.status, replay.body)
	}

	if err := composition.grants.Revoke(composition.sessionID); err != nil {
		t.Fatal(err)
	}
	startsBefore := composition.runner.starts.Load()
	gitStartsBefore := composition.gitStarts.Load()
	for name, tokenAndCall := range map[string]struct {
		token string
		call  string
	}{
		"write": {writeToken, oauthWriteCall(composition.sessionID, "editable.txt", "after\n", "revoked\n")},
		"read":  {readToken, readCall(composition.sessionID, "editable.txt")},
		"test":  {testToken, testWorkspaceCall(composition.sessionID)},
		"git":   {gitToken, gitReviewCall(composition.sessionID)},
	} {
		response := oauthWriteHTTP(t, composition.server, http.MethodPost, "/mcp", "application/json", tokenAndCall.call, tokenAndCall.token, "")
		payload := oauthWriteJSON(t, response)
		if response.status != http.StatusOK {
			t.Fatalf("revoked %s call changed transport status: %d %s", name, response.status, response.body)
		}
		text, isError := readResult(t, payload)
		if !isError || text == "" || (name == "read" && strings.Contains(text, "after\n")) {
			t.Fatalf("revoked %s call was accepted: %s", name, response.body)
		}
	}
	if composition.runner.starts.Load() != startsBefore || composition.gitStarts.Load() != gitStartsBefore {
		t.Fatalf("revoked calls started local operations: test=%d/%d git=%d/%d", composition.runner.starts.Load(), startsBefore, composition.gitStarts.Load(), gitStartsBefore)
	}
	if content := fileText(t, filepath.Join(root, "editable.txt")); content != "after\n" {
		t.Fatalf("revoked write changed workspace: %q", content)
	}

	publicHandler, err := NewOAuthHandler(OAuthConfig{
		ResourceURL:  testResource,
		Issuer:       oauthWriteIssuer,
		OwnerSubject: composition.authorization.OwnerSubject(),
	}, composition.verifier)
	if err != nil {
		t.Fatal(err)
	}
	publicServer := httptest.NewServer(publicHandler)
	t.Cleanup(publicServer.Close)
	publicResponse := oauthWriteHTTP(t, publicServer, http.MethodPost, "/mcp", "application/json", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, testToken, "")
	if publicResponse.status != http.StatusOK {
		t.Fatalf("public tools/list failed: %d %s", publicResponse.status, publicResponse.body)
	}
	publicNames := toolNames(t, oauthWriteJSON(t, publicResponse))
	if len(publicNames) != 1 || publicNames[0] != toolName {
		t.Fatalf("public composition exposed programming tools: %v", publicNames)
	}
	for name, call := range map[string]string{
		"read":  readCall(composition.sessionID, "editable.txt"),
		"write": oauthWriteCall(composition.sessionID, "editable.txt", "after\n", "public\n"),
		"test":  testWorkspaceCall(composition.sessionID),
		"git":   gitReviewCall(composition.sessionID),
	} {
		response := oauthWriteHTTP(t, publicServer, http.MethodPost, "/mcp", "application/json", call, testToken, "")
		payload := oauthWriteJSON(t, response)
		if response.status != http.StatusOK || payload["error"] == nil {
			t.Fatalf("public composition accepted %s tool: %d %s", name, response.status, response.body)
		}
	}
}
