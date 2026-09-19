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
	handler, err := mcp.NewLocalHandler(os.Getenv("SIGNALSPACE_LOCAL_TOKEN"), port)
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

	log.Printf("SignalSpace diagnostic listening at http://127.0.0.1:%d/mcp; OAuth/ChatGPT not implemented", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
