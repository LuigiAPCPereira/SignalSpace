package main

import (
	"bytes"
	"context"
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
