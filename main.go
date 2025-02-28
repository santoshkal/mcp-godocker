// main.go

package main

import (
	"flag"
	"log"

	"santoshkal/mcp-godocker/pkg/server"
	"santoshkal/mcp-godocker/pkg/services"
)

func main() {
	// Define a flag for service selection.
	// When empty, both services (docker and git) will be registered.
	serviceName := flag.String("service", "", "Service to run: docker or git. If not provided, both will be loaded.")
	flag.Parse()

	// Initialize the composite server.
	srv, err := server.NewServer()
	if err != nil {
		log.Fatalf("Error initializing server: %v", err)
	}

	// Register services based on the flag.
	if *serviceName == "" {
		// No service specified, register both.
		dockerService, err := services.NewDockerService()
		if err != nil {
			log.Printf("Docker service not initialized: %v", err)
		} else {
			srv.RegisterService(dockerService)
		}
		gitService := services.NewGitService()
		srv.RegisterService(gitService)
	} else {
		// Only register the specified service.
		switch *serviceName {
		case "docker":
			dockerService, err := services.NewDockerService()
			if err != nil {
				log.Fatalf("Docker service not initialized: %v", err)
			}
			srv.RegisterService(dockerService)
		case "git":
			gitService := services.NewGitService()
			srv.RegisterService(gitService)
		default:
			log.Fatalf("Unsupported service: %s. Please specify either 'docker' or 'git'.", *serviceName)
		}
	}

	// Start the JSON-RPC server.
	srv.StartRPCServer()
}
