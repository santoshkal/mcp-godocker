// ./pkg/mcp/types.go

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

const JSONRPCVersion = "2.0"

// RPCRequest defines the JSON-RPC request structure.
type RPCRequest struct {
	Version string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// RPCResponse defines the JSON-RPC response structure.
type RPCResponse struct {
	Version string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
	ID      *int            `json:"id"`
}

// RPCError defines an error in JSON-RPC responses.
type RPCError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message"`
}

// RPCErrorResponse is another form of error response.
type RPCErrorResponse struct {
	Version  string   `json:"jsonrpc"`
	ErrorObj RPCError `json:"error"`
	ID       *int     `json:"id"`
}

// NewError creates a new RPCError with the given code and message.
func NewError(code int, msg string) *RPCError {
	return &RPCError{
		Code:    code,
		Message: msg,
	}
}

// String returns the string representation of the RPCError.
func (e *RPCError) String() string {
	return fmt.Sprintf("RPC Error [Code: %d]: %s", e.Code, e.Message)
}

// ToolCallArgs represents arguments for directly calling a tool.
type ToolCallArgs struct {
	ToolName   string                 `json:"tool_name"`
	Parameters map[string]interface{} `json:"parameters"`
}

// ToolHandler is the function signature for tool handlers.
type ToolHandler func(ctx context.Context, reg Registry, parameters map[string]interface{}) error

// Registry defines the interface for registering tools.
type Registry interface {
	RegisterTool(name, description string, inputSchema map[string]interface{}, handler ToolHandler)
}
