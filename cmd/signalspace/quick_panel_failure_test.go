package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/tunnel"
)

// O teste força a saída de Serve depois do probe administrativo verdadeiro.
// O processo cloudflared é simulado; os listeners HTTP são os do Quick real.
func TestQuickPanelAdministrativeServerFailureClosesWholeSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	for _, beforePublication := range []bool{true, false} {
		name := "after_publication"
		if beforePublication {
			name = "before_publication"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cloudflared")
			script := "#!/bin/sh\n[ \"$1\" = tunnel ] && [ \"$2\" = --url ] && [ \"$3\" = http://127.0.0.1:7676 ] || exit 2\necho 'https://admin-failure.trycloudflare.com' >&2\nexec sleep 30\n"
			if err := os.WriteFile(path, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var output synchronizedBuffer
			created := make(chan *http.Server, 1)
			verified := make(chan *http.Server, 1)
			finished := make(chan error, 1)
			factory := func(handler http.Handler) *http.Server {
				server := admin.NewServer(handler)
				created <- server
				return server
			}
			verify := func(_ context.Context, resource string) (mcp.TransportReport, error) {
				if resource != "https://admin-failure.trycloudflare.com/mcp" {
					return mcp.TransportReport{}, fmt.Errorf("unexpected resource %q", resource)
				}
				client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second}
				for _, endpoint := range []struct {
					address, host, path string
				}{
					{admin.PublicAddress, "admin-failure.trycloudflare.com", "/.well-known/oauth-protected-resource"},
					{admin.AdminAddress, "localhost:7677", "/api/admin/v1/session"},
				} {
					req, err := http.NewRequest(http.MethodGet, "http://"+endpoint.address+endpoint.path, nil)
					if err != nil {
						return mcp.TransportReport{}, err
					}
					req.Host = endpoint.host
					res, err := client.Do(req)
					if err != nil {
						return mcp.TransportReport{}, err
					}
					_ = res.Body.Close()
					if res.StatusCode != http.StatusOK {
						return mcp.TransportReport{}, fmt.Errorf("readiness HTTP %d for %s", res.StatusCode, endpoint.path)
					}
				}
				server := <-created
				if beforePublication {
					if err := server.Close(); err != nil {
						return mcp.TransportReport{}, err
					}
					return mcp.TransportReport{}, &net.DNSError{Err: "transient DNS", IsTemporary: true}
				}
				verified <- server
				return mcp.TransportReport{ResourceURL: resource}, nil
			}
			go func() {
				finished <- runQuickWithAdminFactory(ctx, strings.NewReader("PUBLICAR PAINEL\n"), &output, tunnel.Start, verify, compositionDiagnostic, true, factory)
			}()
			if !beforePublication {
				var server *http.Server
				select {
				case server = <-verified:
				case err := <-finished:
					t.Fatalf("Quick ended before readiness: %v", err)
				case <-time.After(10 * time.Second):
					t.Fatal("Quick did not verify both listeners")
				}
				deadline := time.After(3 * time.Second)
				for !strings.Contains(output.String(), "Cole no ChatGPT Web") {
					select {
					case err := <-finished:
						t.Fatalf("Quick terminated before announcement: %v", err)
					case <-deadline:
						t.Fatal("Quick did not announce the validated resource")
					case <-time.After(10 * time.Millisecond):
					}
				}
				if err := server.Close(); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-finished:
				if err == nil || !strings.Contains(err.Error(), "administrative HTTP server") {
					t.Fatalf("administrative failure was ignored or misreported: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("Quick kept tunnel alive after administrative server stopped")
			}
			if beforePublication && (strings.Contains(output.String(), "Código de pareamento") || strings.Contains(output.String(), "Cole no ChatGPT Web")) {
				t.Fatal("Quick announced credentials or public readiness after administrative failure")
			}
			for _, address := range []string{admin.PublicAddress, admin.AdminAddress} {
				listener, err := net.Listen("tcp4", address)
				if err != nil {
					t.Fatalf("listener %s remained active after admin failure: %v", address, err)
				}
				_ = listener.Close()
			}
		})
	}
}
