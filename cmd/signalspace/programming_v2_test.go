package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
)

func TestPublicProgrammingV2UsesOneOAuthScopeAndFailsClosedWithoutPanel(t *testing.T) {
	plan, err := planComposition(compositionProgramming)
	if err != nil {
		t.Fatal(err)
	}
	handler, authorization, console, err := embeddedHandlerForPlanWithAuthorizer(readTestResource, filepath.Join(t.TempDir(), "identity"), plan, func() mcp.ProgrammingAuthorizer { return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = console.Close()
		_ = authorization.Close()
	})

	metadata := readRequest(t, handler, http.MethodGet, "/.well-known/oauth-authorization-server", "", "", "", nil)
	if metadata.Code != http.StatusOK || strings.Contains(metadata.Body.String(), "workspace.read") || !strings.Contains(metadata.Body.String(), compositionProgrammingScope) || !strings.Contains(metadata.Body.String(), "refresh_token") {
		t.Fatalf("Programming OAuth metadata is not v2: %d %s", metadata.Code, metadata.Body.String())
	}
	for _, scope := range []string{"signalspace:diagnostic", "signalspace:diagnostic signalspace:workspace.read", "signalspace:workspace.read"} {
		query := "/authorize?client_id=" + strings.Repeat("a", 32) + "&redirect_uri=" + readTestCallback + "&response_type=code&scope=" + strings.ReplaceAll(scope, " ", "%20") + "&resource=" + readTestResource + "&code_challenge=" + strings.Repeat("b", 43) + "&code_challenge_method=S256&state=programming-v2-negative"
		rejected := readRequest(t, handler, http.MethodGet, query, "", "", "", nil)
		if rejected.Code == http.StatusOK {
			t.Fatalf("Programming v2 accepted legacy scope %q", scope)
		}
	}
	registration := fmt.Sprintf(`{"client_name":"Programming v2 test","redirect_uris":[%q],"grant_types":["authorization_code","refresh_token"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	registered := readRequest(t, handler, http.MethodPost, "/register", registration, "application/json", "", nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", registered.Code, registered.Body.String())
	}
	var client struct {
		ID string `json:"client_id"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &client); err != nil || client.ID == "" {
		t.Fatalf("client registration: %v", err)
	}
	token := authorizeClient(t, handler, func(id string) error { return authorization.DecideTerminal(id, true) }, client.ID, compositionProgrammingScope)
	listing := readRequest(t, handler, http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "application/json", token, nil)
	if listing.Code != http.StatusOK || strings.Contains(listing.Body.String(), "workspace.read") || strings.Contains(listing.Body.String(), "git.review") || !strings.Contains(listing.Body.String(), `"scopes":["signalspace:programming"]`) {
		t.Fatalf("Programming tools/list leaked granular scopes: %d %s", listing.Code, listing.Body.String())
	}
	call := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_file","arguments":{"session_id":"` + strings.Repeat("a", 32) + `","path":"file.txt"}}}`
	response := readRequest(t, handler, http.MethodPost, "/mcp", call, "application/json", token, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "LOCAL_APPROVAL_UNAVAILABLE") || !strings.Contains(response.Body.String(), "not executed") {
		t.Fatalf("Programming without local panel did not fail closed: %d %s", response.Code, response.Body.String())
	}
}

func TestProgrammingPreflightPassesAndRejectsDiagnostic(t *testing.T) {
	plan, err := planComposition(compositionProgramming)
	if err != nil {
		t.Fatal(err)
	}
	var handler http.Handler
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	resource := server.URL + "/mcp"
	h, authorization, console, err := embeddedHandlerForPlanWithAuthorizer(resource, filepath.Join(t.TempDir(), "identity"), plan, func() mcp.ProgrammingAuthorizer { return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = console.Close()
		_ = authorization.Close()
	})
	handler = h

	// Preflight with expected composition scope (signalspace:programming) passes.
	report, err := mcp.CheckEmbeddedTransportForScope(context.Background(), resource, plan.expectedCompositionScope, server.Client())
	if err != nil || report.ResourceURL != resource {
		t.Fatalf("Programming preflight failed with expected scope: %v", err)
	}

	// Preflight expecting diagnostic scope on Programming server fails.
	if _, err := mcp.CheckEmbeddedTransport(context.Background(), resource, server.Client()); err == nil {
		t.Fatal("Programming preflight should fail when expecting diagnostic scope")
	}
}

func TestQuickProgrammingPreflightWiringDerivesScopeFromPlan(t *testing.T) {
	plan, err := planComposition(compositionProgramming)
	if err != nil {
		t.Fatal(err)
	}
	if plan.expectedCompositionScope != compositionProgrammingScope {
		t.Fatalf("expected scope %q, got %q", compositionProgrammingScope, plan.expectedCompositionScope)
	}

	diagPlan, err := planComposition(compositionDiagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if diagPlan.expectedCompositionScope != compositionDiagnosticScope {
		t.Fatalf("expected scope %q, got %q", compositionDiagnosticScope, diagPlan.expectedCompositionScope)
	}

	readPlan, err := planComposition(compositionRead)
	if err != nil {
		t.Fatal(err)
	}
	if readPlan.expectedCompositionScope != compositionDiagnosticScope {
		t.Fatalf("expected scope %q, got %q", compositionDiagnosticScope, readPlan.expectedCompositionScope)
	}
}

func TestQuickProgrammingPanelPublishesProgrammingDescription(t *testing.T) {
	var output strings.Builder
	err := runQuickWithOptions(context.Background(), strings.NewReader("CANCELAR\n"), &output, nil, nil, compositionProgramming, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "READ + WRITE + GIT") || !strings.Contains(output.String(), "PUBLICAR PROGRAMAÇÃO PAINEL") {
		t.Fatalf("unexpected prompt: %s", output.String())
	}
}
