package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"santoshkal/mcp-godocker/pkg/mcp"
	"santoshkal/mcp-godocker/pkg/rpcclient"
)

func main() {
	client := rpcclient.NewRPCClient("http://localhost:1234/rpc")
	ctx := context.Background()

	// Example: Call LLM to generate a plan.
	userInstructions := "Pull postgres:latest image"

	// userInstructions := "Generate a plan to create a MySQL container"
	var planJSON string
	if err := client.CallAndParse(ctx, "Server.CallLLM", &planJSON, userInstructions); err != nil {
		log.Fatalf("Error calling Server.CallLLM: %v", err)
	}
	fmt.Printf("Plan JSON: %s\n", planJSON)

	// Validate plan JSON.
	if !json.Valid([]byte(planJSON)) {
		log.Fatalf("Invalid JSON received from Server.CallLLM")
	}

	// Execute the plan.
	var execResp mcp.RPCResponse
	if err := client.CallAndParse(ctx, "Server.ExecutePlan", &execResp, planJSON); err != nil {
		log.Fatalf("Error calling Server.ExecutePlan: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(execResp.Result, &result); err != nil {
		log.Fatalf("Error unmarshalling result: %v", err)
	}
	fmt.Printf("Plan execution result: Status=%s, Message=%s\n", result["status"], result["message"])
}
