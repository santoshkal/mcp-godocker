package main

import (
	"flag"
	"log"
	"os"

	"santoshkal/mcp-godocker/pkg/server"
)

func main() {
	// Optionally allow overriding the config file path.
	configPath := flag.String("config", "", "Path to the configuration YAML file")
	flag.Parse()

	// If a config path is provided, set the environment variable.
	if *configPath != "" {
		os.Setenv("MCP_CONFIG_PATH", *configPath)
	}

	srv, err := server.NewServer()
	if err != nil {
		log.Fatalf("Error initializing server: %v", err)
	}

	srv.StartRPCServer()
}
