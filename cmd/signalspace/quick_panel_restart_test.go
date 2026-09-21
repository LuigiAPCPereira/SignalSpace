package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

// O teste mantém o túnel fora do escopo: ele verifica a composição local que
// o Quick encerra, separando a identidade OAuth persistente das autorizações
// administrativas e de workspace mantidas somente na memória da instância.
func TestQuickInstanceRestartDropsAdministrativeAndWorkspaceAuthorizations(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "identity")
	resource := "https://restart-test.example/mcp"
	const clientID = "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
	const passphrase = "a-long-local-passphrase"

	_, firstAuthorization, firstConsole, err := embeddedHandlerWithWorkspace(resource, stateDir, true)
	if err != nil {
		t.Fatal(err)
	}
	gate, pairingCode, err := admin.NewGate()
	if err != nil {
		_ = firstConsole.Close()
		_ = firstAuthorization.Close()
		t.Fatal(err)
	}

	bootstrap, paired, err := gate.Bootstrap("")
	if err != nil || paired {
		gate.Close()
		_ = firstConsole.Close()
		_ = firstAuthorization.Close()
		t.Fatalf("initial instance was not unpaired: paired=%t err=%v", paired, err)
	}
	adminSession, err := gate.Pair(bootstrap.Cookie, bootstrap.CSRF, pairingCode, passphrase)
	if err != nil {
		gate.Close()
		_ = firstConsole.Close()
		_ = firstAuthorization.Close()
		t.Fatal(err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "approved.txt"), []byte("ephemeral"), 0600); err != nil {
		gate.Close()
		_ = firstConsole.Close()
		_ = firstAuthorization.Close()
		t.Fatal(err)
	}
	workspaceSession, err := firstConsole.grants.Grant(root, clientID)
	if err != nil {
		gate.Close()
		_ = firstConsole.Close()
		_ = firstAuthorization.Close()
		t.Fatal(err)
	}
	if text, err := firstConsole.grants.ReadText("local-owner", clientID, workspaceSession, "approved.txt"); err != nil || text != "ephemeral" {
		t.Fatalf("initial workspace authorization failed: %q %v", text, err)
	}

	// A ordem replica o encerramento do Quick: a concessão e o OAuth fecham
	// antes da Gate, e nenhuma dessas autorizações é gravada no identity.json.
	if err := firstConsole.Close(); err != nil {
		t.Fatal(err)
	}
	if err := firstAuthorization.Close(); err != nil {
		t.Fatal(err)
	}
	gate.Close()

	if _, err := gate.Verify(adminSession.Cookie, adminSession.CSRF, true); !errors.Is(err, admin.ErrAccessDenied) {
		t.Fatalf("administrative session survived instance shutdown: %v", err)
	}
	if _, err := firstConsole.grants.ReadText("local-owner", clientID, workspaceSession, "approved.txt"); !errors.Is(err, workspace.ErrClosed) {
		t.Fatalf("workspace authorization survived instance shutdown: %v", err)
	}

	_, secondAuthorization, secondConsole, err := embeddedHandlerWithWorkspace(resource, stateDir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer secondConsole.Close()
	defer secondAuthorization.Close()

	secondGate, _, err := admin.NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer secondGate.Close()
	if _, paired, err := secondGate.Bootstrap(""); err != nil || paired {
		t.Fatalf("restarted instance inherited administrative pairing: paired=%t err=%v", paired, err)
	}
	private := httptest.NewServer(admin.NewServer(secondGate.Handler()).Handler)
	defer private.Close()
	status, body, cookies := adminHTTP(t, private.Client(), private.URL, "localhost:7677", http.MethodGet, "/api/admin/v1/session", "", "", "", &http.Cookie{
		Name:  "signalspace_admin_session",
		Value: adminSession.Cookie,
	})
	if status != http.StatusUnauthorized || !strings.Contains(body, `"AUTH_REQUIRED"`) {
		t.Fatalf("old administrative cookie authenticated the restarted HTTP instance: %d %s", status, body)
	}
	clearedOldCookie := false
	for _, cookie := range cookies {
		if cookie.Name == "signalspace_admin_session" && cookie.MaxAge < 0 {
			clearedOldCookie = true
			break
		}
	}
	if !clearedOldCookie {
		t.Fatal("restarted HTTP instance did not clear the stale administrative cookie")
	}
	if secondConsole.grants.AllowsClient(clientID) {
		t.Fatal("restarted instance inherited workspace authorization")
	}
	if _, err := secondConsole.grants.ReadText("local-owner", clientID, workspaceSession, "approved.txt"); !errors.Is(err, workspace.ErrNotAuthorized) {
		t.Fatalf("old workspace session was accepted after restart: %v", err)
	}
}
