package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQuickPublicDNSClassifications(t *testing.T) {
	cases := []struct {
		name, payload, want string
	}{
		{"resolved", `{"Status":0,"Question":[{"name":"test-host.trycloudflare.com.","type":1}],"Answer":[{"name":"test-host.trycloudflare.com.","type":1,"data":"104.16.1.2"}]}`, "resolved"},
		{"nxdomain", `{"Status":3,"Question":[{"name":"test-host.trycloudflare.com.","type":1}]}`, "nxdomain"},
		{"no records", `{"Status":0,"Question":[{"name":"test-host.trycloudflare.com.","type":1}]}`, "no_a_record"},
		{"unexpected query", `{"Status":0,"Question":[{"name":"other.example.","type":1}],"Answer":[{"name":"test-host.trycloudflare.com.","type":1,"data":"104.16.1.2"}]}`, "unavailable"},
		{"invalid address", `{"Status":0,"Question":[{"name":"test-host.trycloudflare.com.","type":1}],"Answer":[{"name":"test-host.trycloudflare.com.","type":1,"data":"not-an-ip"}]}`, "no_a_record"},
		{"invalid JSON", `{`, "unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Query().Get("name") != "test-host.trycloudflare.com" || r.URL.Query().Get("type") != "A" {
					t.Error("unexpected DoH query")
				}
				_, _ = w.Write([]byte(tc.payload))
			}))
			defer server.Close()
			got := probeQuickPublicDNS(context.Background(), "https://test-host.trycloudflare.com/mcp", server.Client(), server.URL)
			if got != tc.want {
				t.Fatalf("status=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestQuickPublicDNSRejectsArbitraryHostWithoutQuery(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))
	defer server.Close()
	for _, resource := range []string{"https://example.com/mcp", "https://test-host.trycloudflare.com:8080/mcp", "http://test-host.trycloudflare.com/mcp"} {
		if status := probeQuickPublicDNS(context.Background(), resource, server.Client(), server.URL); status != "unavailable" {
			t.Fatalf("accepted invalid hostname %q: %s", resource, status)
		}
	}
	if called {
		t.Fatal("queried DNS for an unauthorized resource")
	}
}

func TestQuickPublicDNSDoesNotFollowRedirects(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/redirect") {
			called = true
		}
		http.Redirect(w, r, "/redirect", http.StatusFound)
	}))
	defer server.Close()
	if got := probeQuickPublicDNS(context.Background(), "https://test-host.trycloudflare.com/mcp", server.Client(), server.URL); got != "unavailable" || called {
		t.Fatalf("DoH redirect followed: %s, %t", got, called)
	}
}
