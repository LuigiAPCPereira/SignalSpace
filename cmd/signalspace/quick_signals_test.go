package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/tunnel"
)

func TestQuickTransportFailureReportsOnlySanitizedSignals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "cloudflared")
	script := "#!/bin/sh\nprintf '%s\\n' 'INF Registered tunnel connection secret=never-print-me' >&2\nprintf '%s\\n' 'https://signal-sample.trycloudflare.com' >&2\nexec sleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	err := runQuickWith(context.Background(), strings.NewReader("PUBLICAR\n"), &strings.Builder{}, tunnel.Start, func(context.Context, string) (mcp.TransportReport, error) {
		return mcp.TransportReport{}, errors.New("public metadata mismatch")
	})
	if err == nil || !strings.Contains(err.Error(), "registrations=1, registered=true") || !strings.Contains(err.Error(), "public metadata mismatch") || strings.Contains(err.Error(), "never-print-me") {
		t.Fatalf("unsafe or missing failure diagnostics: %v", err)
	}
}
