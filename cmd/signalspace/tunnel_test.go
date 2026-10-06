package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
)

func TestTunnelProgrammingArgs(t *testing.T) {
	if !tunnelProgrammingArgs([]string{"connect", "tunnel", "programming"}) {
		t.Fatal("canonical tunnel programming command was rejected")
	}
	for _, args := range [][]string{
		{"connect", "tunnel"},
		{"connect", "tunnel", "programming", "panel"},
		{"connect", "quick", "programming"},
		{"connect", "tunnel", "read"},
	} {
		if tunnelProgrammingArgs(args) {
			t.Fatalf("unexpected tunnel command accepted: %v", args)
		}
	}
}

func TestTunnelCredentialIsPrivateStableAndDerivesStableClient(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	path1, token1, client1, err := loadOrCreateTunnelCredential(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if token1 == "" || len(client1) != 32 {
		t.Fatalf("unexpected credential identity: token_empty=%t client_len=%d", token1 == "", len(client1))
	}
	info, err := os.Stat(path1)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("credential is not private: mode=%o", info.Mode().Perm())
	}
	path2, token2, client2, err := loadOrCreateTunnelCredential(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if path1 != path2 || token1 != token2 || client1 != client2 {
		t.Fatalf("credential was not stable across reload: %q/%q %q/%q", path1, path2, client1, client2)
	}
}

func TestTunnelCredentialFailsClosedOnLoosePermissionsAndSymlink(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	path, _, _, err := loadOrCreateTunnelCredential(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := loadOrCreateTunnelCredential(stateDir); err == nil {
		t.Fatal("world-readable tunnel credential was accepted")
	}

	stateDir2 := filepath.Join(t.TempDir(), "state")
	dir := filepath.Join(stateDir2, "tunnel")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(target, []byte("0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "programming.token")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := loadOrCreateTunnelCredential(stateDir2); err == nil {
		t.Fatal("symlink tunnel credential was accepted")
	}
}

func TestTunnelProgrammingCancellationHasNoSideEffect(t *testing.T) {
	for _, name := range []string{
		"SIGNALSPACE_AUTH_MODE", "SIGNALSPACE_RESOURCE_URL", "SIGNALSPACE_OAUTH_ISSUER",
		"SIGNALSPACE_JWKS_URL", "SIGNALSPACE_OAUTH_OWNER_SUBJECT", "SIGNALSPACE_LOCAL_TOKEN",
	} {
		t.Setenv(name, "")
	}
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv("SIGNALSPACE_STATE_DIR", stateDir)
	var output strings.Builder
	if err := runTunnelProgramming(context.Background(), strings.NewReader("CANCELAR\n"), &output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "tunnel")); !os.IsNotExist(err) {
		t.Fatalf("cancelled tunnel command created state: %v", err)
	}
	if !strings.Contains(output.String(), "nenhum listener ou segredo foi criado") {
		t.Fatalf("cancellation was not explicit: %s", output.String())
	}
}

func TestTunnelWorkspacePrincipalUsesExistingOwnerSideSelection(t *testing.T) {
	clientID := strings.Repeat("t", 32)
	console := &workspaceConsole{
		clientLabel: "Secure Tunnel",
		issuedClients: func() []auth.ClientInfo {
			return []auth.ClientInfo{{ID: clientID, Name: tunnelClientName}}
		},
	}
	if !console.isIssuedClient(clientID) || console.isIssuedClient(strings.Repeat("x", 32)) {
		t.Fatal("Tunnel principal selection did not use the closed local client set")
	}
	client, ok := console.issuedClient(clientID)
	if !ok || client.Name != tunnelClientName || console.clientLabelText() != "Secure Tunnel" {
		t.Fatalf("unexpected Tunnel client metadata: %+v ok=%t label=%q", client, ok, console.clientLabelText())
	}
}
