// pkg/server/server.go

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"time"

	"github.com/docker/docker/client"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"

	"santoshkal/mcp-godocker/pkg/mcp"
	"santoshkal/mcp-godocker/pkg/utils"
)

// Service defines an interface for a service to register its tools.
type Service interface {
	Name() string
	RegisterTools(s *Server)
}

// ToolHandler defines the function signature for tool execution.
type ToolHandler func(ctx context.Context, s *Server, parameters map[string]interface{}) error

// RegisteredTool holds metadata and the handler for a tool.
type RegisteredTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	Handler     ToolHandler
}

// Server represents the composite server supporting multiple services.
type Server struct {
	dockerClient *client.Client
	llm          *openai.LLM
	tools        map[string]RegisteredTool
	services     map[string]Service
}

// NewServer initializes a new Server instance.
func NewServer() (*Server, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("Docker client initialization failed: %v", err)
		// Allow server initialization without Docker functionality.
		dockerClient = nil
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}
	llm, err := openai.New(openai.WithToken(apiKey), openai.WithModel("gpt-4o"))
	if err != nil {
		return nil, err
	}

	return &Server{
		dockerClient: dockerClient,
		llm:          llm,
		tools:        make(map[string]RegisteredTool),
		services:     make(map[string]Service),
	}, nil
}

// RegisterTool adds an individual tool to the server.
func (s *Server) RegisterTool(name, description string, inputSchema map[string]interface{}, handler ToolHandler) {
	s.tools[name] = RegisteredTool{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
		Handler:     handler,
	}
}

// RegisterService registers a service and lets it add its tools.
func (s *Server) RegisterService(service Service) {
	s.services[service.Name()] = service
	service.RegisterTools(s)
}

// ListTools returns a slice of strings listing all registered tools.
func (s *Server) listTools() []string {
	var toolList []string
	for _, tool := range s.tools {
		toolList = append(toolList, fmt.Sprintf("%s: %s", tool.Name, tool.Description))
	}
	return toolList
}

// ListServices returns a slice of strings listing all registered services.
func (s *Server) listServices() []string {
	var serviceList []string
	for _, svc := range s.services {
		serviceList = append(serviceList, svc.Name())
	}
	return serviceList
}

// JSON-RPC method to list tools.
func (s *Server) ListToolsRPC(args *struct{}, reply *[]string) error {
	*reply = s.listTools()
	return nil
}

// JSON-RPC method to list services.
func (s *Server) ListServicesRPC(args *struct{}, reply *[]string) error {
	*reply = s.listServices()
	return nil
}

// DockerClient returns the Docker client instance.
func (s *Server) DockerClient() *client.Client {
	return s.dockerClient
}

// CallLLM sends user input to the LLM and returns a generated plan.
func (s *Server) CallLLM(args *string, reply *string) error {
	log.Printf("[CallLLM] Received user input: %s", *args)

	var registeredTools []interface{}
	for _, tool := range s.tools {
		registeredTools = append(registeredTools, map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  tool.InputSchema,
		})
	}

	prompt := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, *args),
		llms.TextParts(llms.ChatMessageTypeSystem, utils.GetSystemPrompt()),
	}

	response, err := s.llm.GenerateContent(context.Background(), prompt, nil)
	if err != nil {
		log.Printf("[CallLLM] OpenAI error: %v", err)
		return fmt.Errorf("LLM API error: %w", err)
	}

	if len(response.Choices) == 0 {
		log.Printf("[CallLLM] Empty response from LLM")
		return fmt.Errorf("LLM returned an empty response")
	}

	var plan []map[string]interface{}
	if err := json.Unmarshal([]byte(response.Choices[0].Content), &plan); err != nil {
		log.Printf("[CallLLM] LLM response is not valid JSON: %v", err)
		return fmt.Errorf("LLM returned invalid JSON: %w", err)
	}

	planBytes, err := json.Marshal(plan)
	if err != nil {
		log.Printf("[CallLLM] Failed to marshal plan: %v", err)
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	*reply = string(planBytes)
	log.Printf("[CallLLM] Returning JSON plan: %s", *reply)
	return nil
}

// ExecutePlan processes the plan using the registered tool handlers.
func (s *Server) ExecutePlan(args *string, reply *mcp.RPCResponse) error {
	response := mcp.RPCResponse{Version: mcp.JSONRPCVersion}
	if args == nil || *args == "" {
		response.Error = mcp.NewError(-32602, "ExecutePlan received empty plan")
		*reply = response
		return nil
	}

	log.Printf("[ExecutePlan] Received Plan: %s", *args)

	var plan []map[string]interface{}
	if err := json.Unmarshal([]byte(*args), &plan); err != nil {
		log.Printf("[ExecutePlan] Error unmarshalling JSON: %v", err)
		response.Error = mcp.NewError(-32700, fmt.Sprintf("failed to parse plan JSON: %v", err))
		*reply = response
		return nil
	}

	if len(plan) == 0 {
		log.Printf("[ExecutePlan] No actions found in plan")
		response.Error = mcp.NewError(-32602, "received empty plan from LLM")
		*reply = response
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, action := range plan {
		log.Printf("[ExecutePlan] Processing action: %+v", action)
		actionType, ok := action["action"].(string)
		if !ok || actionType == "" {
			response.Error = mcp.NewError(-32602, "invalid action format")
			*reply = response
			return nil
		}

		parameters, _ := action["parameters"].(map[string]interface{})
		if tool, exists := s.tools[actionType]; exists {
			if err := tool.Handler(ctx, s, parameters); err != nil {
				response.Error = mcp.NewError(-32000, fmt.Sprintf("failed to execute tool %s: %v", actionType, err))
				*reply = response
				return nil
			}
		} else {
			response.Error = mcp.NewError(-32601, fmt.Sprintf("unknown action: %s", actionType))
			*reply = response
			return nil
		}
	}

	result, err := json.Marshal(map[string]string{
		"status":  "success",
		"message": "Plan executed successfully",
	})
	if err != nil {
		response.Error = mcp.NewError(-32000, fmt.Sprintf("failed to marshal result: %v", err))
	} else {
		response.Result = json.RawMessage(result)
	}
	*reply = response
	return nil
}

// CallTool allows direct invocation of a tool.
func (s *Server) CallTool(args *mcp.ToolCallArgs, reply *mcp.RPCResponse) error {
	response := mcp.RPCResponse{Version: mcp.JSONRPCVersion}
	tool, exists := s.tools[args.ToolName]
	if !exists {
		response.Error = mcp.NewError(-32601, fmt.Sprintf("unknown tool: %s", args.ToolName))
		*reply = response
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := tool.Handler(ctx, s, args.Parameters); err != nil {
		response.Error = mcp.NewError(-32000, fmt.Sprintf("failed to execute tool %s: %v", args.ToolName, err))
		*reply = response
		return nil
	}

	result, err := json.Marshal(map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Tool %s executed successfully", args.ToolName),
	})
	if err != nil {
		response.Error = mcp.NewError(-32000, fmt.Sprintf("failed to marshal result: %v", err))
	} else {
		response.Result = json.RawMessage(result)
	}
	*reply = response
	return nil
}

// httpReadWriteCloser adapts HTTP request/response to io.ReadWriteCloser.
type httpReadWriteCloser struct {
	r io.ReadCloser
	w io.Writer
}

func (hrwc *httpReadWriteCloser) Read(p []byte) (int, error) { return hrwc.r.Read(p) }

func (hrwc *httpReadWriteCloser) Write(p []byte) (int, error) { return hrwc.w.Write(p) }

func (hrwc *httpReadWriteCloser) Close() error { return hrwc.r.Close() }

// StartRPCServer starts the JSON-RPC server on port 1234.
func (s *Server) StartRPCServer() {
	rpcServer := rpc.NewServer()
	// All exported methods of Server (including our new ListToolsRPC and ListServicesRPC)
	// are available via the JSON-RPC interface.
	if err := rpcServer.RegisterName("Server", s); err != nil {
		log.Fatalf("Failed to register RPC service: %v", err)
	}

	http.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "JSON-RPC requires POST", http.StatusMethodNotAllowed)
			return
		}
		rpcServer.ServeCodec(jsonrpc.NewServerCodec(&httpReadWriteCloser{
			r: r.Body,
			w: w,
		}))
	})

	log.Println("JSON-RPC server listening on port 1234 (POST /rpc)...")
	log.Fatal(http.ListenAndServe(":1234", nil))
}
