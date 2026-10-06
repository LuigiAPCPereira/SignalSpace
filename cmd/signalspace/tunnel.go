package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/approval"
	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/policy"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

const (
	tunnelOwnerSubject     = "local-owner"
	tunnelClientName       = "OpenAI Secure MCP Tunnel"
	tunnelConfirmation     = "INICIAR PROGRAMAÇÃO TUNNEL"
	tunnelCredentialMaxLen = 128
)

type emptyOAuthRequests struct{}

func (emptyOAuthRequests) ListRequestSnapshots() []auth.RequestSnapshot {
	return []auth.RequestSnapshot{}
}

func (emptyOAuthRequests) GetRequestSnapshot(string) (auth.RequestSnapshot, error) {
	return auth.RequestSnapshot{}, auth.ErrOAuthRequestNotFound
}

func (emptyOAuthRequests) DecideAndSnapshot(string, int, string) (auth.RequestSnapshot, error) {
	return auth.RequestSnapshot{}, auth.ErrOAuthRequestNotFound
}

func tunnelProgrammingArgs(args []string) bool {
	return len(args) == 3 && args[0] == "connect" && args[1] == "tunnel" && args[2] == "programming"
}

// runTunnelProgramming starts only the private SignalSpace side of the OpenAI
// Secure MCP Tunnel. tunnel-client remains a separate operator-owned process:
// it must point its main MCP channel at 127.0.0.1:7676/mcp and inject the
// generated last-hop credential from the printed file reference.
func runTunnelProgramming(ctx context.Context, input io.Reader, output io.Writer) error {
	for _, name := range []string{
		"SIGNALSPACE_AUTH_MODE", "SIGNALSPACE_RESOURCE_URL", "SIGNALSPACE_OAUTH_ISSUER",
		"SIGNALSPACE_JWKS_URL", "SIGNALSPACE_OAUTH_OWNER_SUBJECT", "SIGNALSPACE_LOCAL_TOKEN",
	} {
		if os.Getenv(name) != "" {
			return fmt.Errorf("connect tunnel programming requires %s to be unset", name)
		}
	}
	plan, err := planComposition(compositionProgramming)
	if err != nil {
		return err
	}

	fmt.Fprintln(output, "Secure MCP Tunnel Programming mantém o MCP e o painel somente em loopback. O tunnel-client é operado separadamente e nunca deve apontar para 7677.")
	fmt.Fprintf(output, "Digite %s para iniciar. Qualquer outra resposta cancela.\n", tunnelConfirmation)
	reader := bufio.NewReader(input)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != tunnelConfirmation {
		fmt.Fprintln(output, "Conexão cancelada; nenhum listener ou segredo foi criado.")
		return nil
	}

	stateDir, err := embeddedStateDir()
	if err != nil {
		return err
	}
	credentialPath, token, clientID, err := loadOrCreateTunnelCredential(stateDir)
	if err != nil {
		return fmt.Errorf("initialize Secure MCP Tunnel credential: %w", err)
	}

	ports, err := reserveQuickPortsForPlan(plan, true)
	if err != nil {
		return err
	}
	defer ports.Close()

	gate, pairingCode, err := admin.NewGate()
	if err != nil {
		return fmt.Errorf("initialize local admin authentication: %w", err)
	}
	defer gate.Close()

	grants, err := workspace.NewGrants(tunnelOwnerSubject)
	if err != nil {
		return err
	}
	managed, err := workspace.NewManagedWorktreeManager(stateDir)
	if err != nil {
		_ = grants.Close()
		return err
	}
	clientInfo := auth.ClientInfo{ID: clientID, Name: tunnelClientName}
	console := &workspaceConsole{
		grants: grants, owner: tunnelOwnerSubject, readEnabled: true, managed: managed,
		clientLabel: "Secure Tunnel",
		issuedClients: func() []auth.ClientInfo {
			return []auth.ClientInfo{clientInfo}
		},
	}
	defer console.Close()

	policyStore, err := policy.OpenStore(filepath.Join(stateDir, "policies.json"))
	if err != nil {
		return fmt.Errorf("initialize local capability policies: %w", err)
	}
	capabilityPolicies, err := policy.NewEngineWithStore(time.Now, policyStore, console.managedWorkspaceStable)
	if err != nil {
		return fmt.Errorf("load local capability policies: %w", err)
	}
	console.programmingProfile = standardProgrammingProfile(tunnelOwnerSubject, capabilityPolicies)

	grantApproval, err := workspace.NewCapabilityApproval(grants, console.isIssuedClient)
	if err != nil {
		return fmt.Errorf("initialize Programming workspace approval: %w", err)
	}
	console.programmingApproval = grantApproval
	console.programmingGitReviewer = newWorkspaceGitReviewer(grants, programmingGitOutputLimit)

	approvalConfig := approval.DefaultConfig()
	approvalConfig.BeforeDecision = capabilityPolicies.ApplyApproval
	capabilityApprovals, err := approval.NewWithConfig(approvalConfig)
	if err != nil {
		return fmt.Errorf("initialize capability approvals: %w", err)
	}
	defer capabilityApprovals.Close()

	programmingBridge := &programmingAuthorizer{
		owner: tunnelOwnerSubject, grants: grants, policies: capabilityPolicies, approvals: capabilityApprovals,
	}
	handler, err := mcp.NewTunnelProgrammingHandler(mcp.TunnelProgrammingConfig{
		OwnerSubject: tunnelOwnerSubject,
		ClientID:     clientID,
		Token:        token,
		Port:         7676,
		ProgrammingAuthorization: func() mcp.ProgrammingAuthorizer {
			return programmingBridge
		},
		OnMCPEvent: func(method, diagnosticID string) {
			if method == "tools/list" {
				log.Print("Authenticated Secure Tunnel MCP tool discovery served: tools/list")
			} else if method == "tools/call" {
				log.Printf("Authenticated Secure Tunnel connection_diagnostic handled: diagnosticID=%s", diagnosticID)
			}
		},
	}, mcp.ProgrammingPorts{
		WorkspaceReader:           grants,
		WorkspaceLister:           grants,
		WorkspaceStatter:          grants,
		WorkspaceFinder:           grants,
		WorkspaceSearcher:         grants,
		WorkspaceWriter:           grants,
		WorkspaceDirectoryCreator: grants,
		WorkspaceTextCreator:      grants,
		WorkspaceTextUpdater:      grants,
		WorkspaceCopier:           grants,
		WorkspaceMover:            grants,
		WorkspaceFileDeleter:      grants,
		WorkspaceDirectoryDeleter: grants,
		WorkspacePatchApplier:     grants,
		GitReviewer:               console.programmingGitReviewer,
		GitStatusReader:           console.programmingGitReviewer,
		GitIndexer:                newWorkspaceGitIndexOperator(grants),
		GitCommitter:              newWorkspaceGitCommitter(grants, managed),
	})
	if err != nil {
		return err
	}

	mcpServer := diagnosticServer(handler)
	adminServer := admin.NewServer(gate.HandlerWithRequestsAndCapabilityApprovalsAndPolicies(
		emptyOAuthRequests{}, capabilityApprovals, capabilityPolicies,
	))
	serveDone := make(chan error, 2)
	mcpExited := make(chan struct{})
	adminExited := make(chan struct{})
	go func() {
		serveDone <- fmt.Errorf("Secure Tunnel MCP server: %w", mcpServer.Serve(ports.Public))
		close(mcpExited)
	}()
	go func() {
		serveDone <- fmt.Errorf("administrative HTTP server: %w", adminServer.Serve(ports.Admin))
		close(adminExited)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mcpServer.Shutdown(shutdownCtx)
		_ = adminServer.Shutdown(shutdownCtx)
		_ = mcpServer.Close()
		_ = adminServer.Close()
		<-mcpExited
		<-adminExited
	}()

	if err := awaitQuickAdmin(ctx, ports.Admin, serveDone); err != nil {
		return err
	}
	if err := awaitTunnelMCP(ctx, ports.Public, token, serveDone); err != nil {
		return err
	}

	fmt.Fprintln(output, "SignalSpace Secure MCP Tunnel Programming pronto.")
	fmt.Fprintln(output, "MCP local: http://127.0.0.1:7676/mcp")
	fmt.Fprintf(output, "Header local do tunnel-client: %s: file:%s\n", mcp.TunnelProgrammingHeader, credentialPath)
	fmt.Fprintf(output, "Principal local dedicado: %s\n", clientID)
	fmt.Fprintln(output, "No ChatGPT, use Connection: Tunnel e Authentication: No authentication para este app privado.")
	fmt.Fprintln(output, "Use um tunnel/runtime dedicado ao SignalSpace; não reutilize o principal do outro MCP.")
	fmt.Fprintln(output, "Administração local: http://localhost:7677/ (nunca configure esta porta no tunnel-client).")
	fmt.Fprintf(output, "Código de pareamento desta instância (somente neste terminal, expira em 5 minutos): %s\n", pairingCode)
	fmt.Fprintln(output, "O token do último hop não é OAuth nem grant de workspace; READ/WRITE/Git continuam sujeitos ao grant, profile e approvals locais.")
	fmt.Fprintln(output, "Use workspace clients para obter o principal e workspace request-programming/request-worktree para autorizar um workspace.")

	go serveTerminalCommands(nil, console, reader, output)
	select {
	case <-ctx.Done():
		return nil
	case err := <-serveDone:
		if errors.Is(err, http.ErrServerClosed) && ctx.Err() != nil {
			return nil
		}
		return err
	}
}

func awaitTunnelMCP(ctx context.Context, listener net.Listener, token string, stopped <-chan error) error {
	if listener == nil {
		return errors.New("Secure Tunnel MCP listener missing")
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 350 * time.Millisecond}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	interval := time.NewTicker(25 * time.Millisecond)
	defer interval.Stop()
	address := "http://" + listener.Addr().String() + "/mcp"
	body := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-06-18\"}}")
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, body)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(mcp.TunnelProgrammingHeader, token)
		response, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
			return fmt.Errorf("Secure Tunnel MCP readiness rejected: HTTP %d", response.StatusCode)
		}
		body = strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-06-18\"}}")
		select {
		case stoppedErr := <-stopped:
			return fmt.Errorf("Secure Tunnel MCP stopped before readiness: %w", stoppedErr)
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("Secure Tunnel MCP not ready: %w", err)
		case <-interval.C:
		}
	}
}

func loadOrCreateTunnelCredential(stateDir string) (path, token, clientID string, err error) {
	if !filepath.IsAbs(stateDir) || filepath.Clean(stateDir) != stateDir {
		return "", "", "", errors.New("state directory must be canonical and absolute")
	}
	dir := filepath.Join(stateDir, "tunnel")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", "", err
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 || dirInfo.Mode().Perm()&0077 != 0 {
		return "", "", "", errors.New("tunnel state directory must be a private regular directory")
	}
	path = filepath.Join(dir, "programming.token")
	token, err = readTunnelCredential(path)
	if errors.Is(err, os.ErrNotExist) {
		token, err = createTunnelCredential(path)
	}
	if err != nil {
		return "", "", "", err
	}
	sum := sha256.Sum256([]byte(token))
	clientID = base64.RawURLEncoding.EncodeToString(sum[:24])
	return path, token, clientID, nil
}

func createTunnelCredential(path string) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err := io.WriteString(file, token); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	ok = true
	return token, nil
}

func readTunnelCredential(path string) (string, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Mode().Perm()&0077 != 0 || before.Size() > tunnelCredentialMaxLen {
		return "", errors.New("tunnel credential file must be private and regular")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		return "", errors.New("tunnel credential changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, tunnelCredentialMaxLen+1))
	if err != nil || len(data) > tunnelCredentialMaxLen {
		return "", errors.New("tunnel credential is unavailable or oversized")
	}
	token := string(data)
	if !validTunnelCredential(token) {
		return "", errors.New("tunnel credential is malformed")
	}
	return token, nil
}

func validTunnelCredential(token string) bool {
	if len(token) < 32 || len(token) > tunnelCredentialMaxLen || strings.TrimSpace(token) != token {
		return false
	}
	for _, r := range token {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}
