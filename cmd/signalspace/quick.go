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

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/tunnel"
)

type quickStarter func(context.Context) (*tunnel.Quick, error)
type quickVerifier func(context.Context, string) (mcp.TransportReport, error)

// runQuick só apresenta o endereço quando a URL pública passa no diagnóstico.
func runQuick(ctx context.Context, input io.Reader, output io.Writer) error {
	return runQuickMode(ctx, input, output, compositionDiagnostic)
}

func runQuickMode(ctx context.Context, input io.Reader, output io.Writer, mode compositionMode) error {
	return runQuickModePanel(ctx, input, output, mode, false)
}

func runQuickModePanel(ctx context.Context, input io.Reader, output io.Writer, mode compositionMode, panel bool) error {
	return runQuickWithOptions(ctx, input, output, tunnel.Start, func(ctx context.Context, resource string) (mcp.TransportReport, error) {
		return mcp.CheckEmbeddedTransport(ctx, resource, nil)
	}, mode, panel)
}

func runQuickWith(ctx context.Context, input io.Reader, output io.Writer, start quickStarter, verify quickVerifier) error {
	return runQuickWithMode(ctx, input, output, start, verify, compositionDiagnostic)
}

func runQuickWithMode(ctx context.Context, input io.Reader, output io.Writer, start quickStarter, verify quickVerifier, mode compositionMode) error {
	return runQuickWithOptions(ctx, input, output, start, verify, mode, false)
}

// runQuickWithOptions só liga a API administrativa em modo panel explícito.
// A origem cloudflared permanece fixa em 127.0.0.1:7676, nunca na porta 7677.
func runQuickWithOptions(ctx context.Context, input io.Reader, output io.Writer, start quickStarter, verify quickVerifier, mode compositionMode, panel bool) error {
	return runQuickWithAdminFactory(ctx, input, output, start, verify, mode, panel, admin.NewServer)
}

// runQuickWithAdminFactory permite testar a saída do servidor administrativo.
// A execução normal sempre fornece admin.NewServer; a fábrica não é configurável pela CLI.
func runQuickWithAdminFactory(ctx context.Context, input io.Reader, output io.Writer, start quickStarter, verify quickVerifier, mode compositionMode, panel bool, adminServerFactory func(http.Handler) *http.Server) error {
	plan, err := planComposition(mode)
	if err != nil {
		return err
	}
	for _, name := range []string{"SIGNALSPACE_AUTH_MODE", "SIGNALSPACE_RESOURCE_URL", "SIGNALSPACE_OAUTH_ISSUER", "SIGNALSPACE_JWKS_URL", "SIGNALSPACE_OAUTH_OWNER_SUBJECT", "SIGNALSPACE_LOCAL_TOKEN", "SIGNALSPACE_STATE_DIR"} {
		if os.Getenv(name) != "" {
			return fmt.Errorf("connect quick requires %s to be unset (isolated OAuth state)", name)
		}
	}
	confirmation := "PUBLICAR"
	if plan.workspaceReadScope != "" {
		confirmation = "PUBLICAR LEITURA"
		fmt.Fprintln(output, "Quick Tunnel experimental: a URL ficará pública. read_file poderá ler texto de uma pasta aprovada para um cliente OAuth; nunca use pastas com segredos nesta fase. Sem edição, Git ou shell.")
	} else {
		fmt.Fprintln(output, "Quick Tunnel experimental: a URL ficará pública na internet, somente com connection_diagnostic. Sem arquivos, Git ou terminal.")
	}
	if panel {
		confirmation += " PAINEL"
		fmt.Fprintln(output, "Administração local: http://localhost:7677/ com pareamento local; nunca exponha a porta 7677 por proxy ou túnel.")
	}
	fmt.Fprintf(output, "Digite %s para iniciar o túnel. Qualquer outra resposta cancela.\n", confirmation)
	reader := bufio.NewReader(input)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != confirmation {
		fmt.Fprintln(output, "Conexão cancelada; nenhum túnel iniciado.")
		return nil
	}

	// A reserva conjunta ocorre ANTES de cloudflared. A falha na segunda porta
	// libera a primeira e nunca provoca fallback para o modo terminal.
	ports, err := reserveQuickPorts(panel)
	if err != nil {
		return err
	}
	defer ports.Close()
	var gate *admin.Gate
	var pairingCode string
	if panel {
		gate, pairingCode, err = admin.NewGate()
		if err != nil {
			return fmt.Errorf("initialize local admin authentication: %w", err)
		}
		defer gate.Close()
	}
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
	handler, authorization, console, err := embeddedHandlerForPlan(resource, stateDir, plan)
	if err != nil {
		return err
	}
	defer authorization.Close()
	if plan.consoleMode == workspaceConsoleApprovalsOnly {
		console, err = newWorkspaceConsole(authorization)
		if err != nil {
			return err
		}
	} else if plan.consoleMode != workspaceConsoleRead || console == nil {
		return errors.New("composition plan has no supported workspace console")
	}
	defer console.Close()

	server := diagnosticServer(handler)
	serveDone := make(chan error, 2)
	serverExited := make(chan struct{})
	if panel {
		adminServer := adminServerFactory(gate.HandlerWithRequests(authorization))
		adminExited := make(chan struct{})
		go func() {
			serveDone <- fmt.Errorf("administrative HTTP server: %w", adminServer.Serve(ports.Admin))
			close(adminExited)
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = adminServer.Shutdown(shutdownCtx)
			_ = adminServer.Close()
			<-adminExited
		}()
		if err := awaitQuickAdmin(ctx, ports.Admin, serveDone); err != nil {
			return err
		}
	}
	go func() {
		serveDone <- server.Serve(ports.Public)
		close(serverExited)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		_ = server.Close()
		<-serverExited
	}()

	// A URL pode ser anunciada antes da conexão e do registro DNS público.
	// Evitar uma consulta prematura que envenenaria caches com NXDOMAIN.
	if err := waitQuickDNSWarmup(ctx, quick, 2*time.Second, 3*time.Second); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	if err := awaitQuickTransport(ctx, resource, quick.Done(), serveDone, verify, 60*time.Second, 2*time.Second); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		status := quick.Diagnostics()
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			return fmt.Errorf("%w; public_dns=%s; cloudflared signals: registrations=%d, registered=%t, disconnections=%d, connection_errors=%d, log_read_errors=%d (raw logs omitted)", err, probeQuickPublicDNS(ctx, resource, nil, publicDNSURL), status.Registrations, status.Registered, status.Disconnections, status.ConnectionErrors, status.LogReadErrors)
		}
		return fmt.Errorf("%w; cloudflared signals: registrations=%d, registered=%t, disconnections=%d, connection_errors=%d, log_read_errors=%d (raw logs omitted)", err, status.Registrations, status.Registered, status.Disconnections, status.ConnectionErrors, status.LogReadErrors)
	}
	select {
	case <-quick.Done():
		return errors.New("cloudflared exited before the connection was ready")
	case err := <-serveDone:
		return fmt.Errorf("local HTTP service stopped before publishing the URL: %w", err)
	default:
	}
	if panel {
		fmt.Fprintln(output, "Administração local: http://localhost:7677/ (a API permanece em /api/admin/v1; nunca publique a porta 7677 por proxy ou túnel).")
		fmt.Fprintf(output, "Código de pareamento desta instância (somente neste terminal, expira em 5 minutos): %s\n", pairingCode)
	}
	fmt.Fprintf(output, "Diagnóstico HTTPS aprovado. Cole no ChatGPT Web: %s\n", resource)
	if panel {
		fmt.Fprintln(output, "A API local e o terminal compartilham a mesma decisão OAuth. Nenhum acesso MCP é autorizado por chamada individual; a porta 7677 não é publicada pelo túnel.")
	} else {
		fmt.Fprintln(output, "A autorização requer approve <id> ou deny <id> neste terminal. ChatGPT Web ainda não foi verificado.")
	}
	if plan.workspaceReadScope != "" {
		fmt.Fprintln(output, "Leitura experimental: conclua OAuth de diagnóstico; workspace clients; workspace request <client-id> <absolute-path>; workspace approve <id>. Uma nova autorização OAuth com escopo de leitura é obrigatória; informe o session ID da concessão ao chat somente se quiser usar a ferramenta.")
	} else {
		fmt.Fprintln(output, "Workspace local: workspace clients; workspace request <client-id> <absolute-path> (aprovação posterior; nenhuma leitura MCP habilitada).")
	}
	go serveTerminalCommands(authorization, console, reader, output)
	select {
	case <-ctx.Done():
		return nil
	case <-quick.Done():
		return errors.New("cloudflared disconnected; Quick Tunnel session ended")
	case err := <-serveDone:
		if errors.Is(err, http.ErrServerClosed) && ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("local HTTP service stopped: %w", err)
	}
}

// awaitQuickTransport aguarda apenas erros DNS transitórios; uma falha de
// certificado, contrato OAuth ou desafio MCP continua encerrando a sessão.
func awaitQuickTransport(ctx context.Context, resource string, tunnelDone <-chan struct{}, serverDone <-chan error, verify quickVerifier, window, interval time.Duration) error {
	readyCtx, stop := context.WithTimeout(ctx, window)
	defer stop()
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tunnelDone:
			return errors.New("cloudflared stopped during HTTPS verification")
		case err := <-serverDone:
			return fmt.Errorf("loopback MCP stopped during HTTPS verification: %w", err)
		default:
		}
		checkCtx, cancel := context.WithTimeout(readyCtx, 25*time.Second)
		_, lastErr = verify(checkCtx, resource)
		cancel()
		if lastErr == nil {
			return nil
		}
		var dnsErr *net.DNSError
		if !errors.As(lastErr, &dnsErr) || !(dnsErr.IsNotFound || dnsErr.IsTemporary || dnsErr.IsTimeout) {
			return fmt.Errorf("public HTTPS verification failed, tunnel closed: %w", lastErr)
		}
		// A resposta NXDOMAIN de um hostname recém-criado pode ficar em cache.
		// Nunca alterar o resolvedor do usuário nem desabilitar a validação TLS.
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-tunnelDone:
			timer.Stop()
			return errors.New("cloudflared stopped during HTTPS verification")
		case err := <-serverDone:
			timer.Stop()
			return fmt.Errorf("loopback MCP stopped during HTTPS verification: %w", err)
		case <-readyCtx.Done():
			timer.Stop()
			return fmt.Errorf("public HTTPS verification failed after DNS retry window, tunnel closed: %w", lastErr)
		case <-timer.C:
		}
	}
}

// waitQuickDNSWarmup espera pelo marcador de conexão de forma limitada e concede
// uma curta janela para a criação do registro DNS antes da primeira consulta.
// A ausência do marcador não vira prova de falha: formatos de log podem mudar.
func waitQuickDNSWarmup(ctx context.Context, quick *tunnel.Quick, registrationWindow, settle time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, registrationWindow)
	defer cancel()
	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	for !quick.Diagnostics().Registered {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-quick.Done():
			return errors.New("cloudflared stopped before DNS warm-up")
		case <-waitCtx.Done():
			goto settleDNS
		case <-poll.C:
		}
	}
settleDNS:
	timer := time.NewTimer(settle)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-quick.Done():
		return errors.New("cloudflared stopped during DNS warm-up")
	case <-timer.C:
		return nil
	}
}
