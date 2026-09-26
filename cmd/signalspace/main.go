package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func main() {
	if len(os.Args) != 1 {
		if mode, panel, ok := quickModeArgs(os.Args[1:]); ok {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			if err := runQuickModePanel(ctx, os.Stdin, os.Stdout, mode, panel); err != nil {
				log.Fatal(err)
			}
			return
		}
		if len(os.Args) != 3 || os.Args[1] != "doctor" {
			log.Fatal("usage: signalspace [doctor oauth|transport|connect quick [read|programming] [panel]]")
		}
		var err error
		switch os.Args[2] {
		case "oauth":
			err = runOAuthDoctor(context.Background(), os.Stdout)
		case "transport":
			err = runTransportDoctor(context.Background(), os.Stdout)
		default:
			log.Fatal("usage: signalspace [doctor oauth|transport|connect quick [read|programming] [panel]]")
		}
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	const port = 7676
	var (
		handler http.Handler
		err     error
	)
	if resource := os.Getenv("SIGNALSPACE_RESOURCE_URL"); os.Getenv("SIGNALSPACE_AUTH_MODE") == "embedded" {
		// Nunca reutilizar credenciais de um provedor externo no modo integrado.
		if resource == "" || os.Getenv("SIGNALSPACE_JWKS_URL") != "" || os.Getenv("SIGNALSPACE_OAUTH_ISSUER") != "" || os.Getenv("SIGNALSPACE_OAUTH_OWNER_SUBJECT") != "" || os.Getenv("SIGNALSPACE_LOCAL_TOKEN") != "" {
			log.Fatal("embedded OAuth requires only SIGNALSPACE_RESOURCE_URL and SIGNALSPACE_AUTH_MODE=embedded")
		}
		stateDir, stateErr := embeddedStateDir()
		if stateErr != nil {
			log.Fatal(stateErr)
		}
		var authorization *auth.Server
		var authErr error
		handler, authorization, authErr = embeddedHandler(resource, stateDir)
		if authErr != nil {
			log.Fatal(authErr)
		}
		defer authorization.Close()
		go serveApprovals(authorization, os.Stdin, os.Stdout)
	} else if resource != "" {
		verifier, verifierErr := mcp.NewJWKSVerifier(os.Getenv("SIGNALSPACE_JWKS_URL"))
		if verifierErr != nil {
			log.Fatal(verifierErr)
		}
		handler, err = mcp.NewOAuthHandler(mcp.OAuthConfig{
			ResourceURL:  resource,
			Issuer:       os.Getenv("SIGNALSPACE_OAUTH_ISSUER"),
			OwnerSubject: os.Getenv("SIGNALSPACE_OAUTH_OWNER_SUBJECT"),
		}, verifier)
	} else {
		// Configuração OAuth parcial nunca ativa um fallback de autenticação local.
		if os.Getenv("SIGNALSPACE_AUTH_MODE") != "" || os.Getenv("SIGNALSPACE_JWKS_URL") != "" || os.Getenv("SIGNALSPACE_OAUTH_ISSUER") != "" || os.Getenv("SIGNALSPACE_OAUTH_OWNER_SUBJECT") != "" {
			log.Fatal("SIGNALSPACE_RESOURCE_URL is required for OAuth mode")
		}
		handler, err = mcp.NewLocalHandler(os.Getenv("SIGNALSPACE_LOCAL_TOKEN"), port)
	}
	if err != nil {
		log.Fatal(err)
	}

	server := diagnosticServer(handler)

	log.Printf("SignalSpace diagnostic listening on loopback 127.0.0.1:%d; ChatGPT integration unverified", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// embeddedHandler preserva a composição de diagnóstico sem concessões de arquivos.
func embeddedHandler(resource, stateDir string) (http.Handler, *auth.Server, error) {
	handler, authorization, _, err := embeddedHandlerWithWorkspace(resource, stateDir, compositionDiagnostic)
	return handler, authorization, err
}

// embeddedHandlerWithWorkspace só compõe os modos fechados definidos pela política.
func embeddedHandlerWithWorkspace(resource, stateDir string, mode compositionMode) (http.Handler, *auth.Server, *workspaceConsole, error) {
	plan, err := planComposition(mode)
	if err != nil {
		return nil, nil, nil, err
	}
	return embeddedHandlerForPlanWithAuthorizerMode(resource, stateDir, plan, nil, false)
}

func embeddedHandlerForPlan(resource, stateDir string, plan compositionPlan) (http.Handler, *auth.Server, *workspaceConsole, error) {
	return embeddedHandlerForPlanWithAuthorizerMode(resource, stateDir, plan, nil, false)
}

func embeddedHandlerForPlanWithAuthorizer(resource, stateDir string, plan compositionPlan, programmingAuthorization func() mcp.ProgrammingAuthorizer) (http.Handler, *auth.Server, *workspaceConsole, error) {
	return embeddedHandlerForPlanWithAuthorizerMode(resource, stateDir, plan, programmingAuthorization, true)
}

func embeddedHandlerForPlanWithAuthorizerMode(resource, stateDir string, plan compositionPlan, programmingAuthorization func() mcp.ProgrammingAuthorizer, programmingV2 bool) (http.Handler, *auth.Server, *workspaceConsole, error) {
	closedPlan, err := validateCompositionPlan(plan)
	if err != nil {
		return nil, nil, nil, err
	}
	plan = closedPlan
	issuer := strings.TrimSuffix(resource, "/mcp")
	var grants *workspace.Grants
	var console *workspaceConsole
	oauthBaseScope := compositionDiagnosticScope
	if programmingV2 && plan.mode == compositionProgramming {
		oauthBaseScope = compositionProgrammingScope
	}
	authConfig := auth.Config{ResourceURL: resource, Issuer: issuer, Scope: oauthBaseScope, StateDir: stateDir, OnRequest: func(info auth.RequestInfo) {
		log.Printf("Authorization requested: %s; client: %s; client_id: %s; redirect: %s; scope: %s; type approve %s or deny %s", info.ID, info.Client, info.ClientID, info.Redirect, info.Scope, info.ID, info.ID)
	}, OnRegistrationFailure: func(reason string) {
		// Categoria fixa: não registrar corpos nem credenciais OAuth.
		log.Printf("OAuth client registration rejected: %s (client metadata omitted)", reason)
	}}
	if (!programmingV2 || plan.mode != compositionProgramming) && plan.workspaceReadScope != "" {
		authConfig.ReadScope = plan.workspaceReadScope
		authConfig.CanIssueRead = func(clientID string) bool {
			// Falhar fechado se a composição não terminou ou a concessão foi revogada.
			return grants != nil && grants.AllowsClientScope(clientID, workspace.ScopeRead)
		}
	}
	if (!programmingV2 || plan.mode != compositionProgramming) && plan.workspaceWriteScope != "" {
		authConfig.WriteScope = plan.workspaceWriteScope
		authConfig.CanIssueWrite = func(clientID string) bool {
			return grants != nil && grants.AllowsClientScope(clientID, workspace.ScopeWrite)
		}
	}
	if (!programmingV2 || plan.mode != compositionProgramming) && plan.gitReviewScope != "" {
		authConfig.GitScope = plan.gitReviewScope
		authConfig.CanIssueGit = func(clientID string) bool {
			return grants != nil && grants.AllowsClientScope(clientID, workspace.ScopeGit)
		}
	}
	if (!programmingV2 || plan.mode != compositionProgramming) && plan.gitIndexScope != "" {
		authConfig.GitIndexScope = plan.gitIndexScope
		authConfig.CanIssueGitIndex = func(clientID string) bool {
			return grants != nil && grants.AllowsClientScope(clientID, workspace.ScopeGitIndex)
		}
	}
	if (!programmingV2 || plan.mode != compositionProgramming) && plan.gitCommitScope != "" {
		authConfig.GitCommitScope = plan.gitCommitScope
		authConfig.CanIssueGitCommit = func(clientID string) bool {
			return grants != nil && grants.AllowsClientScope(clientID, workspace.ScopeGitCommit)
		}
	}
	if programmingV2 && plan.mode == compositionProgramming {
		authConfig.CompositionScope = compositionProgrammingScope
		authConfig.EnableRefreshTokens = true
	}
	authorization, err := auth.New(authConfig)
	if err != nil {
		return nil, nil, nil, err
	}
	closeFailure := func(err error) (http.Handler, *auth.Server, *workspaceConsole, error) {
		if console != nil {
			_ = console.Close()
		} else if grants != nil {
			_ = grants.Close()
		}
		_ = authorization.Close()
		return nil, nil, nil, err
	}
	if plan.consoleMode == workspaceConsoleRead || plan.consoleMode == workspaceConsoleProgramming {
		grants, err = workspace.NewGrants(authorization.OwnerSubject())
		if err != nil {
			return closeFailure(err)
		}
		managed, managedErr := workspace.NewManagedWorktreeManager(stateDir)
		if managedErr != nil {
			return closeFailure(managedErr)
		}
		console = &workspaceConsole{grants: grants, owner: authorization.OwnerSubject(), issuedClients: authorization.IssuedClients, readEnabled: true, managed: managed}
		if plan.consoleMode == workspaceConsoleProgramming {
			approval, approvalErr := workspace.NewCapabilityApproval(grants, func(clientID string) bool {
				return console.isIssuedClient(clientID)
			})
			if approvalErr != nil {
				return closeFailure(approvalErr)
			}
			console.programmingApproval = approval
			console.programmingGitReviewer = newWorkspaceGitReviewer(grants, programmingGitOutputLimit)
		}
	}
	var verifier *mcp.JWKSVerifier
	switch plan.validatorMode {
	case compositionLocalOAuthJWTValidator:
		verifier, err = mcp.NewStaticJWTVerifier(authorization.PublicKey(), authorization.KeyID())
	default:
		return closeFailure(errors.New("composition plan has no supported local OAuth validator"))
	}
	if err != nil {
		return closeFailure(err)
	}
	mcpConfig := mcp.OAuthConfig{ResourceURL: resource, Issuer: issuer, OwnerSubject: authorization.OwnerSubject(), ProgrammingAuthorization: programmingAuthorization, OnMCPEvent: func(method, diagnosticID string) {
		// Registrar somente método conhecido e ID aleatório; sem token ou argumentos.
		if method == "tools/list" {
			log.Print("Authenticated MCP tool discovery served: tools/list")
		} else if method == "tools/call" {
			log.Printf("Authenticated MCP connection_diagnostic handled: diagnosticID=%s (caller identity not attested)", diagnosticID)
		}
	}}
	if plan.workspaceReadScope != "" {
		// Ambas as ferramentas usam a mesma concessão revogável do terminal.
		mcpConfig.WorkspaceReader = grants
		mcpConfig.WorkspaceLister = grants
	}
	var protected http.Handler
	if plan.consoleMode == workspaceConsoleProgramming {
		programmingHandler := mcp.NewOAuthProgrammingHandler
		if programmingV2 {
			programmingHandler = mcp.NewOAuthProgrammingV2Handler
		}
		protected, err = programmingHandler(mcpConfig, mcp.ProgrammingPorts{
			WorkspaceReader:           mcpConfig.WorkspaceReader,
			WorkspaceLister:           mcpConfig.WorkspaceLister,
			WorkspaceStatter:          grants,
			WorkspaceFinder:           grants,
			WorkspaceSearcher:         grants,
			WorkspaceWriter:           console.grants,
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
			GitIndexer:                newWorkspaceGitIndexOperator(console.grants),
			GitCommitter:              newWorkspaceGitCommitter(console.grants, console.managed),
		}, verifier)
	} else {
		protected, err = mcp.NewOAuthHandler(mcpConfig, verifier)
	}
	if err != nil {
		return closeFailure(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", protected)
	mux.Handle("/.well-known/oauth-protected-resource", protected)
	mux.Handle("/.well-known/oauth-protected-resource/mcp", protected)
	for _, path := range []string{"/.well-known/oauth-authorization-server", "/oauth/jwks", "/register", "/authorize", "/authorize/consent.js", "/authorize/complete", "/token"} {
		mux.Handle(path, authorization.Handler())
	}
	// Consulta pública apenas do próprio pedido OAuth; nenhum handler administrativo.
	mux.Handle("/authorize/status", authorization.PublicStatusHandler())
	return rejectPublicAdministrativePaths(mux), authorization, console, nil
}

// rejectPublicAdministrativePaths rejeita segmentos administrativos antes que
// o ServeMux normalize a rota pública e responda com redirecionamento.
func rejectPublicAdministrativePaths(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, segment := range strings.Split(r.URL.Path, "/") {
			if segment != "admin" {
				continue
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serveApprovals não oferece endpoints públicos para aceitar solicitações.
func serveApprovals(authorization *auth.Server, input io.Reader, output io.Writer) {
	serveTerminalCommands(authorization, nil, input, output)
}

// serveTerminalCommands recebe apenas stdin local; concessões não são rotas HTTP.
func serveTerminalCommands(authorization *auth.Server, console *workspaceConsole, input io.Reader, output io.Writer) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 8192)
	for scanner.Scan() {
		line := scanner.Text()
		if console != nil && console.handleWorkspaceCommand(line, output) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || (fields[0] != "approve" && fields[0] != "deny") {
			fmt.Fprintln(output, "use approve <id> or deny <id>")
			continue
		}
		// O terminal usa a mesma transição atômica e a mesma revalidação de
		// concessão exigidas pela API administrativa; stdin não é um atalho.
		if err := authorization.DecideTerminal(fields[1], fields[0] == "approve"); err != nil {
			fmt.Fprintln(output, err)
		} else {
			fmt.Fprintln(output, "authorization decision recorded")
		}
	}
}

func diagnosticServer(handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              admin.PublicAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
}

// embeddedStateDir concentra a configuração do armazenamento privado no entrypoint.
func embeddedStateDir() (string, error) {
	dir := os.Getenv("SIGNALSPACE_STATE_DIR")
	if dir == "" {
		base := os.Getenv("XDG_STATE_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".local", "state")
		}
		dir = filepath.Join(base, "signalspace")
	}
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return "", errors.New("SIGNALSPACE_STATE_DIR must be a canonical absolute path")
	}
	return dir, nil
}

// runOAuthDoctor verifica os metadados sem solicitar login ou iniciar o servidor MCP.
func runOAuthDoctor(ctx context.Context, out io.Writer) error {
	resource := os.Getenv("SIGNALSPACE_RESOURCE_URL")
	issuer := os.Getenv("SIGNALSPACE_OAUTH_ISSUER")
	jwksURL := os.Getenv("SIGNALSPACE_JWKS_URL")
	owner := os.Getenv("SIGNALSPACE_OAUTH_OWNER_SUBJECT")
	verifier, err := mcp.NewJWKSVerifier(jwksURL)
	if err != nil {
		return err
	}
	if _, err := mcp.NewOAuthHandler(mcp.OAuthConfig{
		ResourceURL:  resource,
		Issuer:       issuer,
		OwnerSubject: owner,
	}, verifier); err != nil {
		return err
	}
	report, err := mcp.CheckOAuthProvider(ctx, issuer, jwksURL, nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "OAuth metadata: verified (%s)\n", report.MetadataURL)
	fmt.Fprintf(out, "PKCE S256 and authorization code: advertised\n")
	fmt.Fprintf(out, "JWKS: reachable and parseable (%s)\n", report.JWKSURL)
	fmt.Fprintf(out, "Client registration: %s\n", report.Registration)
	fmt.Fprintln(out, "NOT VERIFIED: resource parameter propagation, owner login/consent, client registration, HTTPS tunnel and ChatGPT Web invocation.")
	return nil
}

// runTransportDoctor consulta a URL pública configurada sem abrir o servidor nem enviar segredos.
func runTransportDoctor(ctx context.Context, out io.Writer) error {
	if os.Getenv("SIGNALSPACE_AUTH_MODE") != "embedded" || os.Getenv("SIGNALSPACE_RESOURCE_URL") == "" ||
		os.Getenv("SIGNALSPACE_JWKS_URL") != "" || os.Getenv("SIGNALSPACE_OAUTH_ISSUER") != "" ||
		os.Getenv("SIGNALSPACE_OAUTH_OWNER_SUBJECT") != "" || os.Getenv("SIGNALSPACE_LOCAL_TOKEN") != "" {
		return errors.New("transport doctor requires only SIGNALSPACE_AUTH_MODE=embedded and SIGNALSPACE_RESOURCE_URL")
	}
	report, err := mcp.CheckEmbeddedTransport(ctx, os.Getenv("SIGNALSPACE_RESOURCE_URL"), nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Public HTTPS resource: reachable (%s)\n", report.ResourceURL)
	fmt.Fprintf(out, "OAuth issuer and metadata: consistent (%s)\n", report.Issuer)
	fmt.Fprintln(out, "JWKS and unauthenticated MCP challenges: verified")
	fmt.Fprintln(out, "NOT VERIFIED: owner login/consent, real ChatGPT OAuth callback, token exchange and ChatGPT Web invocation.")
	return nil
}
