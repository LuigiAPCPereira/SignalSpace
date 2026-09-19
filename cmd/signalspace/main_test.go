package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestOAuthDoctorRejectsMissingConfiguration(t *testing.T) {
	t.Setenv("SIGNALSPACE_RESOURCE_URL", "https://signalspace.example/mcp")
	t.Setenv("SIGNALSPACE_OAUTH_ISSUER", "https://identity.example/")
	t.Setenv("SIGNALSPACE_JWKS_URL", "https://identity.example/jwks")
	t.Setenv("SIGNALSPACE_OAUTH_OWNER_SUBJECT", "")
	var output bytes.Buffer
	if err := runOAuthDoctor(context.Background(), &output); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("missing owner accepted: %v", err)
	}
	if output.Len() != 0 {
		t.Fatal("doctor printed a successful result with invalid configuration")
	}
}

func TestEmbeddedStateDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", base)
	t.Setenv("SIGNALSPACE_STATE_DIR", "")
	got, err := embeddedStateDir()
	if err != nil || got != filepath.Join(base, "signalspace") {
		t.Fatalf("invalid XDG state directory: %q, %v", got, err)
	}
	t.Setenv("SIGNALSPACE_STATE_DIR", "relative/state")
	if _, err := embeddedStateDir(); err == nil {
		t.Fatal("relative private state directory accepted")
	}
	t.Setenv("SIGNALSPACE_STATE_DIR", filepath.Join(base, "private"))
	if got, err := embeddedStateDir(); err != nil || got != filepath.Join(base, "private") {
		t.Fatalf("explicit state directory not respected: %q, %v", got, err)
	}
}
