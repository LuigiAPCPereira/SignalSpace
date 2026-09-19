package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/tunnel"
)

type quickStarter func(context.Context) (*tunnel.Quick, error)
type quickVerifier func(context.Context, string) (mcp.TransportReport, error)

// runQuick só apresenta o endereço quando a URL pública passa no diagnóstico.
func runQuick(ctx context.Context, input io.Reader, output io.Writer) error {
	return runQuickWith(ctx, input, output, tunnel.Start, func(ctx context.Context, resource string) (mcp.TransportReport, error) {
		return mcp.CheckEmbeddedTransport(ctx, resource, nil)
	})
}

func runQuickWith(ctx context.Context, input io.Reader, output io.Writer, start quickStarter, verify quickVerifier) error {
	for _, name := range []string{"SIGNALSPACE_AUTH_MODE", "SIGNALSPACE_RESOURCE_URL", "SIGNALSPACE_OAUTH_ISSUER", "SIGNALSPACE_JWKS_URL", "SIGNALSPACE_OAUTH_OWNER_SUBJECT", "SIGNALSPACE_LOCAL_TOKEN", "SIGNALSPACE_STATE_DIR"} {
		if os.Getenv(name) != "" {
			return fmt.Errorf("connect quick requires %s to be unset (isolated OAuth state)", name)
		}
	}
	fmt.Fprintln(output, "Quick Tunnel experimental: a URL ficará pública na internet, somente com connection_diagnostic. Sem arquivos, Git ou terminal.")
	fmt.Fprintln(output, "Digite PUBLICAR para iniciar o túnel. Qualquer outra resposta cancela.")
	reader := bufio.NewReader(input)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != "PUBLICAR" {
		fmt.Fprintln(output, "Conexão cancelada; nenhum túnel iniciado.")
		return nil
	}

	// Reservar a porta antes de publicar: não expor outro serviço por engano.
	listener, err := net.Listen("tcp", "127.0.0.1:7676")
	if err != nil {
		return fmt.Errorf("bind diagnostic loopback: %w", err)
	}
	defer listener.Close()
	quick, err := start(ctx)
	if err != nil {
		return err
	}
	defer quick.Close()

	// Cada subdomínio aleatório usa chave e clientes separados; nenhuma credencial permanente é alterada.
	stateDir, err := os.MkdirTemp("", "signalspace-quick-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stateDir)
	resource := quick.URL + "/mcp"
	handler, authorization, err := embeddedHandler(resource, stateDir)
	if err != nil {
		return err
	}
	defer authorization.Close()
	server := diagnosticServer(handler)
	serveDone := make(chan error, 1)
	serverExited := make(chan struct{})
	go func() {
		serveDone <- server.Serve(listener)
		close(serverExited)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		_ = server.Close()
		<-serverExited
	}()

	// A publicação não é declarada pronta apenas porque cloudflared imprimiu uma URL.
	var lastErr error
	ready := false
	for attempt := 0; attempt < 3; attempt++ {
		select {
		case <-ctx.Done():
			return nil
		case <-quick.Done():
			return errors.New("cloudflared stopped during HTTPS verification")
		case err := <-serveDone:
			return fmt.Errorf("loopback MCP stopped during HTTPS verification: %w", err)
		default:
		}
		checkCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		_, lastErr = verify(checkCtx, resource)
		cancel()
		if lastErr == nil {
			ready = true
			break
		}
		if attempt != 2 {
			select {
			case <-ctx.Done():
				return nil
			case <-quick.Done():
				return errors.New("cloudflared stopped during HTTPS verification")
			case <-time.After(time.Second):
			}
		}
	}
	if !ready {
		return fmt.Errorf("public HTTPS verification failed, tunnel closed: %w", lastErr)
	}
	select {
	case <-quick.Done():
		return errors.New("cloudflared exited before the connection was ready")
	default:
	}
	fmt.Fprintf(output, "Diagnóstico HTTPS aprovado. Cole no ChatGPT Web: %s\n", resource)
	fmt.Fprintln(output, "A autorização requer approve <id> ou deny <id> neste terminal. ChatGPT Web ainda não foi verificado.")
	go serveApprovals(authorization, reader, output)
	select {
	case <-ctx.Done():
		return nil
	case <-quick.Done():
		return errors.New("cloudflared disconnected; Quick Tunnel session ended")
	case err := <-serveDone:
		if errors.Is(err, http.ErrServerClosed) && ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("local MCP server stopped: %w", err)
	}
}
