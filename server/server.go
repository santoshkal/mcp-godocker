package main

import (
	"context"
	"encoding/json"
	"errors"
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

	"santoshkal/mcp-godocker/pkg/docker"
	"santoshkal/mcp-godocker/pkg/llm"
	"santoshkal/mcp-godocker/pkg/mcp"
	"santoshkal/mcp-godocker/utils"
)

// ToolHandler defines the function signature for tool execution.
type ToolHandler func(ctx context.Context, s *Server, parameters map[string]interface{}) error

// RegisteredTool holds metadata and the handler for a tool.
type RegisteredTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	Handler     ToolHandler
}

// Server encapsulates the Docker client, LLM client, and a registry of tools.
type Server struct {
	dockerClient *client.Client
	llmClient    *llm.LLMClient
	tools        map[string]RegisteredTool
}

// NewServer creates and configures a new Server.
func NewServer() (*Server, error) {
	dc, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	llmClient, err := llm.NewLLMClient(apiKey, "gpt-4o")
	if err != nil {
		return nil, err
	}

	s := &Server{
		dockerClient: dc,
		llmClient:    llmClient,
		tools:        make(map[string]RegisteredTool),
	}

	// Register Docker operation tools.
	s.RegisterTool("create_network", "Create a Docker network", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the network",
			},
		},
		"required": []string{"name"},
	}, func(ctx context.Context, s *Server, params map[string]interface{}) error {
		name, _ := params["name"].(string)
		return docker.CreateNetwork(ctx, s.dockerClient, name)
	})

	s.RegisterTool("create_container", "Create a Docker container", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the container",
			},
			"image": map[string]interface{}{
				"type":        "string",
				"description": "Docker image to use",
			},
		},
		"required": []string{"name", "image"},
	}, func(ctx context.Context, s *Server, params map[string]interface{}) error {
		name, _ := params["name"].(string)
		image, _ := params["image"].(string)
		if name == "" || image == "" {
			return errors.New("missing container name or image")
		}
		return docker.CreateContainer(ctx, s.dockerClient, name, image)
	})

	s.RegisterTool("create_volume", "Create a Docker volume", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the volume",
			},
		},
		"required": []string{"name"},
	}, func(ctx context.Context, s *Server, params map[string]interface{}) error {
		name, _ := params["name"].(string)
		return docker.CreateVolume(ctx, s.dockerClient, name)
	})

	s.RegisterTool("run_container", "Run (start) a Docker container", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the container",
			},
		},
		"required": []string{"name"},
	}, func(ctx context.Context, s *Server, params map[string]interface{}) error {
		name, _ := params["name"].(string)
		return docker.RunContainer(ctx, s.dockerClient, name)
	})

	s.RegisterTool("pull_image", "Pull a Docker image", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"image": map[string]interface{}{
				"type":        "string",
				"description": "Combined image name (e.g. mysql:latest)",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Image name",
			},
			"tag": map[string]interface{}{
				"type":        "string",
				"description": "Image tag",
			},
		},
		"required": []string{"image"},
	}, func(ctx context.Context, s *Server, params map[string]interface{}) error {
		return docker.PullImage(ctx, s.dockerClient, params)
	})

	return s, nil
}

// RegisterTool adds a new tool to the server's registry.
func (s *Server) RegisterTool(name, description string, inputSchema map[string]interface{}, handler ToolHandler) {
	s.tools[name] = RegisteredTool{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
		Handler:     handler,
	}
}

// CallLLM sends user instructions to the LLM and returns a generated plan (JSON).
func (s *Server) CallLLM(args *string, reply *string) error {
	log.Printf("[CallLLM] Received user input: %s", *args)
	var registeredTools []llms.Tool
	for _, tool := range s.tools {
		registeredTools = append(registeredTools, llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}
	prompt := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, *args),
		llms.TextParts(llms.ChatMessageTypeSystem, utils.GetSystemPrompt()),
	}
	response, err := s.llmClient.GeneratePlan(context.Background(), prompt, registeredTools)
	if err != nil {
		log.Printf("[CallLLM] OpenAI error: %v", err)
		return fmt.Errorf("CallLLM OpenAI API error: %w", err)
	}
	if len(response.Choices) == 0 {
		return fmt.Errorf("CallLLM received an empty response from OpenAI")
	}
	var plan []map[string]interface{}
	if err := json.Unmarshal([]byte(response.Choices[0].Content), &plan); err != nil {
		log.Printf("[CallLLM] LLM response is not valid JSON: %v", err)
		return fmt.Errorf("CallLLM returned invalid JSON: %w", err)
	}
	planBytes, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("CallLLM failed to marshal plan: %w", err)
	}
	*reply = string(planBytes)
	log.Printf("[CallLLM] Returning JSON plan: %s", *reply)
	return nil
}

// ExecutePlan processes and executes the plan using the registered tool handlers.
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
		response.Error = mcp.NewError(-32700, fmt.Sprintf("failed to parse plan JSON: %v", err))
		*reply = response
		return nil
	}
	if len(plan) == 0 {
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

// CallTool allows direct invocation of an individual tool.
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

// HTTP adapter for net/rpc/jsonrpc.
type httpReadWriteCloser struct {
	r io.ReadCloser
	w io.Writer
}

func (hrwc *httpReadWriteCloser) Read(p []byte) (int, error) { return hrwc.r.Read(p) }

func (hrwc *httpReadWriteCloser) Write(p []byte) (int, error) { return hrwc.w.Write(p) }

func (hrwc *httpReadWriteCloser) Close() error { return hrwc.r.Close() }

// StartRPCServer starts the JSON-RPC server on port 1234.
func StartRPCServer() {
	srv, err := NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("Server", srv); err != nil {
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

func main() {
	// Optionally, generate and log a system prompt here using pkg/mcp/prompt.go.
	StartRPCServer()
	// Keep the main goroutine alive.
	select {}
}
