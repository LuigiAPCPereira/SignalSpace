package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
)

func TestQuickTransportRetriesDNSUntilReady(t *testing.T) {
	attempts := 0
	dnsErr := fmt.Errorf("protected resource metadata: %w", &net.DNSError{Err: "no such host", IsNotFound: true})
	err := awaitQuickTransport(context.Background(), "https://example.trycloudflare.com/mcp", make(chan struct{}), make(chan error), func(context.Context, string) (mcp.TransportReport, error) {
		attempts++
		if attempts < 3 {
			return mcp.TransportReport{}, dnsErr
		}
		return mcp.TransportReport{}, nil
	}, time.Second, time.Millisecond)
	if err != nil || attempts != 3 {
		t.Fatalf("DNS recovery not verified: attempts=%d err=%v", attempts, err)
	}
}

func TestQuickTransportFailsClosedWithoutRetryOnContractError(t *testing.T) {
	attempts := 0
	err := awaitQuickTransport(context.Background(), "https://example.trycloudflare.com/mcp", make(chan struct{}), make(chan error), func(context.Context, string) (mcp.TransportReport, error) {
		attempts++
		return mcp.TransportReport{}, errors.New("protected resource metadata mismatch")
	}, time.Second, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "metadata mismatch") || attempts != 1 {
		t.Fatalf("permanent error retried or accepted: attempts=%d err=%v", attempts, err)
	}
}

func TestQuickTransportDNSRetryDeadline(t *testing.T) {
	attempts := 0
	err := awaitQuickTransport(context.Background(), "https://example.trycloudflare.com/mcp", make(chan struct{}), make(chan error), func(context.Context, string) (mcp.TransportReport, error) {
		attempts++
		return mcp.TransportReport{}, &net.DNSError{Err: "no such host", IsNotFound: true}
	}, 25*time.Millisecond, 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "after DNS retry window") || attempts < 2 {
		t.Fatalf("DNS wait did not expire safely: attempts=%d err=%v", attempts, err)
	}
}

func TestQuickTransportStopsWaitingForTunnelOrCancellation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause string
	}{
		{"tunnel", "cloudflared stopped"},
		{"context", "context canceled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tunnelDone := make(chan struct{})
			started := make(chan struct{})
			var release chan struct{}
			if tc.name == "context" {
				release = make(chan struct{})
			}
			finished := make(chan error, 1)
			go func() {
				finished <- awaitQuickTransport(ctx, "https://example.trycloudflare.com/mcp", tunnelDone, make(chan error), func(context.Context, string) (mcp.TransportReport, error) {
					close(started)
					if release != nil {
						<-release
					}
					return mcp.TransportReport{}, &net.DNSError{Err: "no such host", IsNotFound: true}
				}, time.Minute, time.Minute)
			}()
			<-started
			if tc.name == "tunnel" {
				close(tunnelDone)
			} else {
				cancel()
				close(release)
			}
			select {
			case err := <-finished:
				if err == nil || !strings.Contains(err.Error(), tc.cause) {
					t.Fatalf("wrong early exit: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("DNS retry did not stop after tunnel exit or cancellation")
			}
		})
	}
}
