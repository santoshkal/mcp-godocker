package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// JSON-RPC Request/Response Types
type RPCRequest struct {
	Version string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type RPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
	ID     int             `json:"id"`
}

type RPCError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// RPCClient: Encapsulates HTTP + JSON-RPC behavior
type RPCClient struct {
	httpClient *http.Client
	endpoint   string
}

// NewRPCClient creates a new client with a configurable endpoint.
func NewRPCClient(endpoint string) *RPCClient {
	return &RPCClient{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		endpoint: endpoint,
	}
}

// Call performs a JSON-RPC call and returns the raw JSON result.
func (c *RPCClient) Call(ctx context.Context, method string, params ...interface{}) (json.RawMessage, error) {
	reqBody := RPCRequest{
		Version: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBuffer(data))
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

	var rpcResp RPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return rpcResp.Result, nil
}

// CallAndParse unmarshals the JSON result into the `out` parameter.
func (c *RPCClient) CallAndParse(ctx context.Context, method string, out interface{}, params ...interface{}) error {
	rawResult, err := c.Call(ctx, method, params...)
	if err != nil {
		return err
	}

	fmt.Printf("[Client] Received JSON: %s\n", string(rawResult))

	if err := json.Unmarshal(rawResult, out); err != nil {
		return fmt.Errorf("failed to parse result into %T: %w", out, err)
	}
	return nil
}

// Main: Demonstrate how to call our server's methods in a "best practice" flow
func main() {
	// 1) Create a client to talk to your JSON-RPC server at :1234/rpc
	client := NewRPCClient("http://localhost:1234/rpc")
	ctx := context.Background()

	// Let's request the server’s LLM to generate a Docker plan:
	//    Server.CallLLM has signature: (args *string, reply *string) error
	//    so we pass a single string argument with instructions, and expect a string back.
	userInstructions := "Generate a plan to create a MySQL container"
	// Request a Docker plan from LLM
	//  Request a Docker plan from LLM
	var planJSON string
	err := client.CallAndParse(ctx, "Server.CallLLM", &planJSON, userInstructions)
	if err != nil {
		fmt.Printf("❌ Error calling Server.CallLLM: %v\n", err)
		return
	}

	fmt.Printf("✅ Received Plan JSON from LLM: %s\n", planJSON)

	// Validate if planJSON is actually JSON
	if !json.Valid([]byte(planJSON)) {
		fmt.Printf("❌ Received invalid JSON from Server.CallLLM\n")
		return
	}

	// Now we execute the plan with "Server.ExecutePlan"
	type ExecutePlanReply struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	var execReply ExecutePlanReply

	err = client.CallAndParse(ctx, "Server.ExecutePlan", &execReply, planJSON)
	if err != nil {
		fmt.Printf("❌ Error calling Server.ExecutePlan: %v\n", err)
		return
	}

	fmt.Printf("✅ Plan execution result: Status=%s, Message=%s\n", execReply.Status, execReply.Message)
}
