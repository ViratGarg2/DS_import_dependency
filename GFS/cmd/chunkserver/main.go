package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/Mit-Vin/GFS-Distributed-Systems/internal/chunkserver"
)

func main() {
	// Parse command-line arguments
	port := flag.Int("port", 8080, "Port number to run the chunk server on")
	host := flag.String("host", "localhost", "Advertised host/IP for this chunk server (must be reachable by clients and peers)")
	listenHost := flag.String("listen-host", "0.0.0.0", "Host/interface to bind the chunk server listener")
	configPath := flag.String("config", "../../configs/chunkserver-config.yml", "Path to chunkserver configuration file")
	flag.Parse()

	// Validate the provided port number
	if *port <= 0 || *port > 65535 {
		log.Fatalf("Invalid port number: %d. Please provide a port between 1 and 65535.\n", *port)
	}

	if strings.TrimSpace(*host) == "" {
		log.Fatal("Invalid host: host cannot be empty")
	}
	if strings.TrimSpace(*listenHost) == "" {
		log.Fatal("Invalid listen-host: listen-host cannot be empty")
	}

	advertisedAddress := net.JoinHostPort(*host, fmt.Sprintf("%d", *port))
	listenAddress := net.JoinHostPort(*listenHost, fmt.Sprintf("%d", *port))
	serverID := fmt.Sprintf("server-%s-%d", sanitizeServerIDPart(*host), *port)
	log.Printf("Initializing chunk server with ID: %s (listen=%s, advertise=%s)\n", serverID, listenAddress, advertisedAddress)

	// Load server configuration
	config, err := chunkserver.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration file: %v\n", err)
	}

	// Verify if the port is available
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("Port %d is not available on %s: %v\n", *port, *listenHost, err)
	}
	// The listener is not used further; defer closing to clean up resources
	listener.Close()
	log.Printf("Port %d is available. Proceeding to initialize the chunk server.\n", *port)

	// Create the chunk server instance
	cs, err := chunkserver.NewChunkServer(serverID, listenAddress, advertisedAddress, config)
	if err != nil {
		log.Fatalf("Failed to create chunk server: %v\n", err)
	}

	// Start the chunk server
	log.Printf("Starting chunk server with ID: %s\n", serverID)
	if err := cs.Start(); err != nil {
		log.Fatalf("Failed to start chunk server: %v\n", err)
	}

	// Keep the program running indefinitely
	log.Printf("Chunk server with ID %s is now running.\n", serverID)
	select {}
}

func sanitizeServerIDPart(host string) string {
	sanitized := strings.TrimSpace(host)
	replacer := strings.NewReplacer(".", "-", ":", "-", "/", "-", "\\", "-")
	sanitized = replacer.Replace(sanitized)
	if sanitized == "" {
		return "unknown"
	}
	return sanitized
}
