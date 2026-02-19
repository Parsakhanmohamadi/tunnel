package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"customtunnel/internal/tunnel"
)

func main() {
	var (
		listenAddr = flag.String("listen", ":8443", "TLS listen address for tunnel server")
		configPath = flag.String("config", "tunnel-server.yaml", "path to server config file")
	)
	flag.Parse()

	srv, err := tunnel.NewServer(*configPath, *listenAddr)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
	log.Printf("tunnel server listening on %s", *listenAddr)

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down server...")
	if err := srv.Close(); err != nil {
		log.Printf("error during shutdown: %v", err)
	}
}

