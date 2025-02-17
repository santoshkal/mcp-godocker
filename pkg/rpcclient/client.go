package rpcclient

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
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		endpoint:   endpoint,
	}
}

// Call performs a JSON-RPC call and returns the raw result.
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
	var rpcResp mcp.RPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf(rpcResp.Error.String())
	}
	return rpcResp.Result, nil
}

// CallAndParse unmarshals the result into out.
func (c *RPCClient) CallAndParse(ctx context.Context, method string, out interface{}, params ...interface{}) error {
	result, err := c.Call(ctx, method, params...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(result, out); err != nil {
		return fmt.Errorf("failed to parse result into %T: %w", out, err)
	}
	return nil
}
