package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

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
	if resource := os.Getenv("SIGNALSPACE_RESOURCE_URL"); resource != "" {
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
		if os.Getenv("SIGNALSPACE_JWKS_URL") != "" || os.Getenv("SIGNALSPACE_OAUTH_ISSUER") != "" || os.Getenv("SIGNALSPACE_OAUTH_OWNER_SUBJECT") != "" {
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
