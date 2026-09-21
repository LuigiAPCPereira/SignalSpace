package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/tunnel"
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

func TestQuickDeclineDoesNotStartProcess(t *testing.T) {
	var out bytes.Buffer
	called := false
	err := runQuickWith(context.Background(), strings.NewReader("CANCELAR\n"), &out, func(context.Context) (*tunnel.Quick, error) {
		called = true
		return nil, errors.New("unexpected tunnel startup")
	}, nil)
	if err != nil || called || !strings.Contains(out.String(), "Conexão cancelada") {
		t.Fatalf("decline started tunnel: err=%v called=%t output=%q", err, called, out.String())
	}
}

func TestQuickRejectsExistingIdentityConfiguration(t *testing.T) {
	t.Setenv("SIGNALSPACE_RESOURCE_URL", "https://permanent.example/mcp")
	var out bytes.Buffer
	err := runQuickWith(context.Background(), strings.NewReader("PUBLICAR\n"), &out, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "SIGNALSPACE_RESOURCE_URL") || out.Len() != 0 {
		t.Fatalf("permanent credentials were not isolated: %v %q", err, out.String())
	}
}

func TestQuickIntegrationLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "cloudflared")
	script := "#!/bin/sh\n[ \"$1\" = tunnel ] && [ \"$2\" = --url ] && [ \"$3\" = http://127.0.0.1:7676 ] || exit 2\necho 'https://test-host.trycloudflare.com' >&2\nexec sleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out synchronizedBuffer
	verified := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- runQuickWith(ctx, strings.NewReader("PUBLICAR\n"), &out, tunnel.Start, func(_ context.Context, resource string) (mcp.TransportReport, error) {
			if resource != "https://test-host.trycloudflare.com/mcp" {
				return mcp.TransportReport{}, fmt.Errorf("unexpected resource: %s", resource)
			}
			req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:7676/.well-known/oauth-protected-resource", nil)
			if err != nil {
				return mcp.TransportReport{}, err
			}
			req.Host = "test-host.trycloudflare.com"
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return mcp.TransportReport{}, err
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != 200 || !bytes.Contains(body, []byte(resource)) {
				return mcp.TransportReport{}, fmt.Errorf("unhealthy OAuth metadata: %d %s", response.StatusCode, body)
			}
			close(verified)
			return mcp.TransportReport{ResourceURL: resource}, nil
		})
	}()
	select {
	case <-verified:
	case err := <-finished:
		t.Fatalf("quick connection ended early: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("quick test did not reach protected origin")
	}
	deadline := time.After(3 * time.Second)
	for !strings.Contains(out.String(), "Cole no ChatGPT Web") {
		select {
		case err := <-finished:
			t.Fatalf("quick connection stopped without publishing a ready URL: %v", err)
		case <-deadline:
			t.Fatal("quick did not print verified URL")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("quick shutdown failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("quick shutdown left a running process or HTTP listener")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:7676")
	if err != nil {
		t.Fatalf("quick leaked the diagnostic listener: %v", err)
	}
	listener.Close()
}

type synchronizedBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}
