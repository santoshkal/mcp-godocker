package main

import (
	"fmt"
	"log"
	"net/rpc/jsonrpc"

	"santoshkal/mcp-godocker/types" // Import the shared package
)

func main() {
	// Connect to the RPC server.
	client, err := jsonrpc.Dial("tcp", "localhost:1234")
	if err != nil {
		log.Fatalf("Failed to connect to RPC server: %v", err)
	}
	defer client.Close()

	// Call ListPrompts RPC method.
	var prompts []types.Prompt
	err = client.Call("Server.ListPrompts", struct{}{}, &prompts)
	if err != nil {
		log.Fatalf("Failed to call ListPrompts: %v", err)
	}
	fmt.Println("Available Prompts:", prompts)

	// Call CallLLM RPC method.
	var plan string
	userInput := "Create a MySQL and WordPress setup"
	err = client.Call("Server.CallLLM", &userInput, &plan)
	if err != nil {
		log.Fatalf("Failed to call CallLLM: %v", err)
	}
	fmt.Println("Generated Plan:", plan)

	// Call ExecutePlan RPC method.
	var result string
	err = client.Call("Server.ExecutePlan", &plan, &result)
	if err != nil {
		log.Fatalf("Failed to call ExecutePlan: %v", err)
	}
	fmt.Println("Execution Result:", result)
}
