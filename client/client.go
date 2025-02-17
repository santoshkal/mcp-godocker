// client.go

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"santoshkal/mcp-godocker/pkg/mcp"
)

type RPCClient struct {
	httpClient *http.Client
	endpoint   string
}

func NewRPCClient(endpoint string) *RPCClient {
	return &RPCClient{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		endpoint: endpoint,
	}
}

// Call performs a JSON-RPC call and returns the raw result from the RPCResponse.
func (c *RPCClient) Call(ctx context.Context, method string, params ...interface{}) (json.RawMessage, error) {
	reqBody := mcp.RPCRequest{
		Version: mcp.JSONRPCVersion,
		Method:  method,
		Params:  params,
		ID:      1,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	fmt.Printf("[Client] Raw JSON-RPC Response: %s\n", string(body))

	var rpcResp mcp.RPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf(rpcResp.Error.String(), rpcResp.Error)
	}

	return rpcResp.Result, nil
}

// CallAndParse unmarshals the JSON result into the provided output parameter.
func (c *RPCClient) CallAndParse(ctx context.Context, method string, out interface{}, params ...interface{}) error {
	result, err := c.Call(ctx, method, params...)
	if err != nil {
		return err
	}
	fmt.Printf("[Client] Received JSON: %s\n", string(result))

	if err := json.Unmarshal(result, out); err != nil {
		return fmt.Errorf("failed to parse result into %T: %w", out, err)
	}
	return nil
}

func main() {
	client := NewRPCClient("http://localhost:1234/rpc")
	ctx := context.Background()

	// Step 1: Get a plan from the LLM.
	userInstructions := "Generate a plan to create a MySQL container"
	// userInstructions := "pull a latest nginx:latest image"
	var planJSON string
	err := client.CallAndParse(ctx, "Server.CallLLM", &planJSON, userInstructions)
	if err != nil {
		fmt.Printf("❌ Error calling Server.CallLLM: %v\n", err)
		return
	}

	fmt.Printf("✅ Received Plan JSON from LLM: %s\n", planJSON)

	// Validate that we got valid JSON.
	if !json.Valid([]byte(planJSON)) {
		fmt.Printf("❌ Received invalid JSON from Server.CallLLM\n")
		return
	}

	// Step 2: Execute the entire plan.
	var execResp mcp.RPCResponse
	err = client.CallAndParse(ctx, "Server.ExecutePlan", &execResp, planJSON)
	if err != nil {
		fmt.Printf("❌ Error calling Server.ExecutePlan: %v\n", err)
		return
	}

	// Unmarshal the result for display.
	var result map[string]string
	if err := json.Unmarshal(execResp.Result, &result); err != nil {
		fmt.Printf("❌ Failed to unmarshal execution result: %v\n", err)
		return
	}
	fmt.Printf("✅ Plan execution result: Status=%s, Message=%s\n", result["status"], result["message"])
}
