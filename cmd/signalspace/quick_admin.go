package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// awaitQuickAdmin exige que a API local responda antes de anunciar a URL pública.
// O Host continua canônico mesmo quando o socket usa o endereço IPv4 de bind.
// A consulta inicial cria somente um bootstrap não privilegiado, sem sessão.
func awaitQuickAdmin(ctx context.Context, listener net.Listener, stopped <-chan error) error {
	if listener == nil {
		return fmt.Errorf("administrative listener missing")
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 350 * time.Millisecond}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	interval := time.NewTicker(25 * time.Millisecond)
	defer interval.Stop()
	address := "http://" + listener.Addr().String() + "/api/admin/v1/session"
	for {
		select {
		case err := <-stopped:
			return fmt.Errorf("administrative server stopped before readiness: %w", err)
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return err
		}
		req.Host = "localhost:7677"
		response, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
			return fmt.Errorf("administrative readiness rejected: HTTP %d", response.StatusCode)
		}
		select {
		case stoppedErr := <-stopped:
			return fmt.Errorf("administrative server stopped before readiness: %w", stoppedErr)
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("administrative server not ready: %w", err)
		case <-interval.C:
		}
	}
}
