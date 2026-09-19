package auth

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestPublicEndpointQuotas(t *testing.T) {
	config := durableConfig(filepath.Join(t.TempDir(), "state"))
	s, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := s.Handler()
	for _, tc := range []struct {
		path   string
		method string
		limit  int
	}{
		{"/register", "POST", 16},
		{"/authorize", "GET", 64},
		{"/authorize/complete", "POST", 128},
		{"/token", "POST", 128},
	} {
		for i := 0; i < tc.limit; i++ {
			response := invoke(h, tc.method, tc.path, "", "", nil)
			if response.Code == http.StatusTooManyRequests {
				t.Fatalf("%s denied before quota: %d", tc.path, i)
			}
		}
		response := invoke(h, tc.method, tc.path, "", "", nil)
		if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s quota response: %d %+v", tc.path, response.Code, response.Header())
		}
		// Consultas de descoberta continuam disponíveis durante excesso de DCR.
		if w := invoke(h, "GET", "/.well-known/oauth-authorization-server", "", "", nil); w.Code != 200 {
			t.Fatal("OAuth discovery was blocked by endpoint quota")
		}
		s.mu.Lock()
		quota := s.quotas[tc.path]
		quota.Started = time.Now().Add(-quotaWindow)
		s.quotas[tc.path] = quota
		s.mu.Unlock()
		if w := invoke(h, tc.method, tc.path, "", "", nil); w.Code == http.StatusTooManyRequests {
			t.Fatalf("%s quota did not reset", tc.path)
		}
	}
}

func TestQuotaHasBoundedKeys(t *testing.T) {
	s, err := New(durableConfig(filepath.Join(t.TempDir(), "state")))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 1000; i++ {
		if !s.allowPublicRequest("/unrelated/"+string(rune(i)), time.Now()) {
			t.Fatal("unrelated route was limited")
		}
	}
	if len(s.quotas) != 0 {
		t.Fatal("attacker-controlled paths allocated rate-limit state")
	}
}
