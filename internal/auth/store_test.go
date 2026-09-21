package auth

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
)

func durableConfig(dir string) Config {
	return Config{ResourceURL: resourceURL, Issuer: "https://signalspace.example", Scope: scope, StateDir: dir}
}

func TestIdentitySurvivesRestartButRequestsDoNot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	config := durableConfig(dir)
	first, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	keyID := first.KeyID()
	modulus := first.PublicKey().N.String()
	clientID := register(t, first.Handler())
	if len(first.clients) != 1 {
		t.Fatal("client was not registered")
	}
	// Uma aprovação pendente não sobrevive ao reinício.
	first.pending["temporary"] = pending{}
	first.codes["temporary"] = grant{}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.KeyID() != keyID || second.PublicKey().N.String() != modulus {
		t.Fatal("signing identity rotated after restart")
	}
	if _, ok := second.clients[clientID]; !ok || len(second.clients) != 1 {
		t.Fatal("registered client lost after restart")
	}
	if len(second.pending) != 0 || len(second.codes) != 0 {
		t.Fatal("ephemeral OAuth grants survived restart")
	}
	if _, _, _ = requestConsent(t, second.Handler(), clientID); len(second.pending) != 1 {
		t.Fatal("registered client could not authorize after restart")
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("state directory is not private: %v", err)
	}
	info, err = os.Stat(filepath.Join(dir, stateFileName))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private key has unsafe permissions: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lock")); err != nil {
		t.Fatal("missing state lock")
	}
}

func TestIdentityRejectsConcurrentInstance(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	config := durableConfig(dir)
	first, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(config); err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("concurrent instance was accepted: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := New(config)
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	defer second.Close()
}

func TestIdentityFailsClosedOnUnsafeStorage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	config := durableConfig(dir)
	first, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	identity := filepath.Join(dir, stateFileName)
	if _, err := New(Config{ResourceURL: "https://changed.example/mcp", Issuer: "https://changed.example", Scope: scope, StateDir: dir}); err == nil {
		t.Fatal("changed resource URL reused the old identity")
	}
	if err := os.Chmod(identity, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(config); err == nil {
		t.Fatal("world-readable private key accepted")
	}
	if err := os.Chmod(identity, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := New(config); err == nil {
		t.Fatal("publicly accessible state directory accepted")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identity, []byte("corrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(config); err == nil {
		t.Fatal("corrupted identity was silently replaced")
	}
}

func TestIdentityRejectsSymlinksAndRegistrationSaveFailure(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "state")
	config := durableConfig(dir)
	first, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	identity := filepath.Join(dir, stateFileName)
	backup := filepath.Join(dir, "backup")
	if err := os.Rename(identity, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backup, identity); err != nil {
		t.Fatal(err)
	}
	// Nenhum registro é emitido se o estado foi alterado de forma insegura.
	response := invoke(first.Handler(), "POST", "/register", `{"client_name":"ChatGPT","redirect_uris":["`+callback+`"]}`, "application/json", nil)
	if response.Code != 503 || len(first.clients) != 0 {
		t.Fatalf("registration bypassed failed storage: %d", response.Code)
	}
	first.Close()
	if _, err := New(config); err == nil {
		t.Fatal("identity symlink accepted on restart")
	}
	if err := os.Remove(identity); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, identity); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dir, filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(durableConfig(filepath.Join(base, "alias"))); err == nil {
		t.Fatal("state directory symlink accepted")
	}
	if second, err := New(config); err != nil {
		t.Fatalf("safe state could not be reopened: %v", err)
	} else {
		defer second.Close()
	}
}

func TestIdentityRejectsRelativeDirectory(t *testing.T) {
	if _, err := New(durableConfig("relative/state")); err == nil {
		t.Fatal("relative state path accepted")
	}
}

func TestPreviouslyIssuedTokenRemainsVerifiableAfterRestart(t *testing.T) {
	first, handler, events := startAuth(t)
	id := register(t, handler)
	_, cookie, csrf := requestConsent(t, handler, id)
	request := <-events
	if err := first.Approve(request.ID, true); err != nil {
		t.Fatal(err)
	}
	callbackURL, err := url.Parse(complete(handler, request.ID, csrf, cookie).Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	issued := redeem(handler, id, callbackURL.Query().Get("code"), testVerifier, resourceURL)
	if issued.Code != 200 {
		t.Fatalf("token request failed: %d", issued.Code)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(issued.Body.Bytes(), &payload); err != nil || payload.AccessToken == "" {
		t.Fatal("missing access token")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := New(durableConfig(first.store.dir))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	verifier, err := mcp.NewStaticJWTVerifier(second.PublicKey(), second.KeyID())
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(context.Background(), payload.AccessToken, second.config.Issuer, resourceURL, scope, second.OwnerSubject()); err != nil {
		t.Fatalf("issued token invalid after restart: %v", err)
	}
}
