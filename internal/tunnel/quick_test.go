package tunnel

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExtractQuickURL(t *testing.T) {
	for _, tc := range []struct {
		line string
		want string
	}{
		{`INF Your quick Tunnel has been created! Visit it at: https://yellow-frog-9.trycloudflare.com`, "https://yellow-frog-9.trycloudflare.com"},
		{`{"level":"info","url":"https://another-link.trycloudflare.com"}`, "https://another-link.trycloudflare.com"},
		{`https://bad.trycloudflare.com.attacker.com`, ""},
		{`https://bad.trycloudflare.com/mcp`, ""},
		{`https://bad.trycloudflare.com:8080`, ""},
		{`https://bad.trycloudflare.com@attacker.com`, ""},
		{`https://attacker.com`, ""},
		{`https://-bad.trycloudflare.com`, ""},
		{`https://bad-.trycloudflare.com`, ""},
	} {
		if got := extractQuickURL(tc.line); got != tc.want {
			t.Errorf("extractQuickURL(%q) = %q; want %q", tc.line, got, tc.want)
		}
	}
}

func quickScript(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "cloudflared")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestQuickProcessLifecycle(t *testing.T) {
	binary := quickScript(t, `
[ "$1" = tunnel ] && [ "$2" = --url ] && [ "$3" = http://127.0.0.1:7676 ] || exit 2
printf '%s\n' '2026 INF https://harmless-seed.trycloudflare.com' >&2
exec sleep 30
`)
	q, err := start(context.Background(), binary, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if q.URL != "https://harmless-seed.trycloudflare.com" {
		t.Fatalf("unexpected URL: %s", q.URL)
	}
	if err := q.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-q.Done():
	default:
		t.Fatal("cloudflared process remained alive")
	}
	if err := q.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestQuickFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"exit", "exit 1\n"},
		{"untrusted-url", "echo https://attacker.example >&2; exit 1\n"},
		{"timeout", "exec sleep 30\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binary := quickScript(t, tc.body)
			_, err := start(context.Background(), binary, 120*time.Millisecond)
			if err == nil || strings.Contains(err.Error(), "attacker.example") {
				t.Fatalf("unexpected failure: %v", err)
			}
		})
	}
}

func TestQuickCancellation(t *testing.T) {
	binary := quickScript(t, "exec sleep 30\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := start(ctx, binary, time.Second)
	if err == nil {
		t.Fatal("cancelled startup was accepted")
	}
}
