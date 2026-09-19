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
	"path/filepath"
	"strings"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
)

func main() {
	if len(os.Args) != 1 {
		if len(os.Args) != 3 || os.Args[1] != "doctor" || os.Args[2] != "oauth" {
			log.Fatal("usage: signalspace [doctor oauth]")
		}
		if err := runOAuthDoctor(context.Background(), os.Stdout); err != nil {
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
		issuer := strings.TrimSuffix(resource, "/mcp")
		stateDir, stateErr := embeddedStateDir()
		if stateErr != nil {
			log.Fatal(stateErr)
		}
		authorization, authErr := auth.New(auth.Config{ResourceURL: resource, Issuer: issuer, Scope: "signalspace:diagnostic", StateDir: stateDir, OnRequest: func(info auth.RequestInfo) {
			log.Printf("Authorization requested: %s; client: %s; redirect: %s; type approve %s or deny %s", info.ID, info.Client, info.Redirect, info.ID, info.ID)
		}})
		if authErr != nil {
			log.Fatal(authErr)
		}
		defer authorization.Close()
		verifier, verifierErr := mcp.NewStaticJWTVerifier(authorization.PublicKey(), authorization.KeyID())
		if verifierErr != nil {
			log.Fatal(verifierErr)
		}
		protected, protectErr := mcp.NewOAuthHandler(mcp.OAuthConfig{ResourceURL: resource, Issuer: issuer, OwnerSubject: authorization.OwnerSubject()}, verifier)
		if protectErr != nil {
			log.Fatal(protectErr)
		}
		mux := http.NewServeMux()
		mux.Handle("/mcp", protected)
		mux.Handle("/.well-known/oauth-protected-resource", protected)
		mux.Handle("/.well-known/oauth-protected-resource/mcp", protected)
		for _, path := range []string{"/.well-known/oauth-authorization-server", "/oauth/jwks", "/register", "/authorize", "/authorize/complete", "/token"} {
			mux.Handle(path, authorization.Handler())
		}
		handler = mux
		// O único canal que decide aprovações é o terminal do proprietário.
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				fields := strings.Fields(scanner.Text())
				if len(fields) != 2 || (fields[0] != "approve" && fields[0] != "deny") {
					log.Print("use approve <id> or deny <id>")
					continue
				}
				if err := authorization.Approve(fields[1], fields[0] == "approve"); err != nil {
					log.Print(err)
				} else {
					log.Print("authorization decision recorded")
				}
			}
		}()
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

	server := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	log.Printf("SignalSpace diagnostic listening on loopback 127.0.0.1:%d; ChatGPT integration unverified", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
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
