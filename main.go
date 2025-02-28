// main.go

package main

import (
	"log"

	"santoshkal/mcp-godocker/pkg/server"
	"santoshkal/mcp-godocker/pkg/services"
)

func main() {
	// Initialize the composite server.
	srv, err := server.NewServer()
	if err != nil {
		log.Fatalf("Error initializing server: %v", err)
	}

	// Register Docker service if available.
	dockerService, err := services.NewDockerService()
	if err != nil {
		log.Printf("Docker service not initialized: %v", err)
	} else {
		srv.RegisterService(dockerService)
	}

	// Register Git service.
	gitService := services.NewGitService()
	srv.RegisterService(gitService)

	// Start the JSON-RPC server.
	srv.StartRPCServer()
}
