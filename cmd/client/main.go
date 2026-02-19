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
		serverAddr = flag.String("server", "server.example.com:8443", "tunnel server address")
		configPath = flag.String("config", "tunnel-client.yaml", "path to client config file")
	)
	flag.Parse()

	cl, err := tunnel.NewClient(*configPath, *serverAddr)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	if err := cl.Start(); err != nil {
		log.Fatalf("failed to start client: %v", err)
	}
	log.Printf("tunnel client connected to %s", *serverAddr)

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down client...")
	if err := cl.Close(); err != nil {
		log.Printf("error during shutdown: %v", err)
	}
}

