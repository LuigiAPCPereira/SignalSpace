package tunnel

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestQuickSignalsFromProcessWithoutRawLogs(t *testing.T) {
	binary := quickScript(t, `
printf '%s\n' 'INF Registered tunnel connection connIndex=0 secret=private-value' >&2
printf '%s\n' 'INF https://signal-sample.trycloudflare.com' >&2
exec sleep 30
`)
	q, err := start(context.Background(), binary, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	status := q.Diagnostics()
	if !status.Registered || status.Registrations != 1 || status.ConnectionErrors != 0 {
		t.Fatalf("registration signal lost: %+v", status)
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(q.URL)), "private-value") {
		t.Fatal("raw logs leaked into public URL")
	}
}

func TestQuickSignalsOnlyKnownMarkers(t *testing.T) {
	q := &Quick{}
	q.observe("INF Your quick Tunnel has been created! Visit it at https://signal-sample.trycloudflare.com")
	if got := q.Diagnostics(); got.Registered || got.Registrations != 0 {
		t.Fatalf("URL incorrectly treated as connected: %+v", got)
	}
	q.observe("INF Registered tunnel connection connIndex=0 secret=never-print-me")
	q.observe("ERR Failed to serve quic connection ip=private-value")
	q.observe("INF Connection terminated connIndex=0")
	if got := q.Diagnostics(); got.Registered || got.Registrations != 1 || got.Disconnections != 1 || got.ConnectionErrors != 1 {
		t.Fatalf("unexpected connection signal counts: %+v", got)
	}
}
