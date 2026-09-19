package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
)

func main() {
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
			ResourceURL: resource,
			Issuer:      os.Getenv("SIGNALSPACE_OAUTH_ISSUER"),
		}, verifier)
	} else {
		// Configuração OAuth parcial nunca ativa um fallback de autenticação local.
		if os.Getenv("SIGNALSPACE_JWKS_URL") != "" || os.Getenv("SIGNALSPACE_OAUTH_ISSUER") != "" {
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
