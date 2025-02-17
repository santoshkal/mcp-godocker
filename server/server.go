// server.go

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

	"github.com/docker/docker/api/types/container"
	img "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"

	"santoshkal/mcp-godocker/pkg/mcp"
	"santoshkal/mcp-godocker/utils"
)

// -----------------------------------------------------------------------------
// Tool registration types and helper methods
// -----------------------------------------------------------------------------

// ToolHandler defines the function signature for tool execution.
type ToolHandler func(ctx context.Context, s *Server, parameters map[string]interface{}) error

// RegisteredTool holds metadata and the handler for a tool.
type RegisteredTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	Handler     ToolHandler
}

// -----------------------------------------------------------------------------
// Server definition including tool registry
// -----------------------------------------------------------------------------

// Server represents the Docker server application.
type Server struct {
	dockerClient *client.Client
	llm          *openai.LLM
	tools        map[string]RegisteredTool
}

// NewServer creates a new Server instance and registers built-in tools.
func NewServer() (*Server, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}
	llm, err := openai.New(openai.WithToken(apiKey), openai.WithModel("gpt-4o"))
	if err != nil {
		return nil, err
	}

	s := &Server{
		dockerClient: dockerClient,
		llm:          llm,
		tools:        make(map[string]RegisteredTool),
	}

	// Register built-in Docker operation tools.
	s.RegisterTool("create_network", "Create a Docker network", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the network",
			},
		},
		"required": []string{"name"},
	}, createNetworkHandler)

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
	}, createContainerHandler)

	s.RegisterTool("create_volume", "Create a Docker volume", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the volume",
			},
		},
		"required": []string{"name"},
	}, createVolumeHandler)

	s.RegisterTool("run_container", "Run (start) a Docker container", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the container",
			},
		},
		"required": []string{"name"},
	}, runContainerHandler)

	s.RegisterTool("pull_image", "Pull a Docker image", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"image": map[string]interface{}{
				"type":        "string",
				"description": "Name of the image to pull",
			},
		},
		"required": []string{"image"},
	}, pullImageHandler)

	return s, nil
}

// RegisterTool adds a new tool with its metadata and handler to the server.
func (s *Server) RegisterTool(name, description string, inputSchema map[string]interface{}, handler ToolHandler) {
	s.tools[name] = RegisteredTool{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
		Handler:     handler,
	}
}

// -----------------------------------------------------------------------------
// LLM and Plan Execution Methods
// -----------------------------------------------------------------------------

// CallLLM sends user input to the OpenAI LLM and returns the generated plan (JSON).
func (s *Server) CallLLM(args *string, reply *string) error {
	log.Printf("[CallLLM] Received user input: %s", *args)

	// Generate a list of tools from the registered tools.
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

	// Build the prompt using the user instruction and system prompt.
	prompt := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, *args),
		llms.TextParts(llms.ChatMessageTypeSystem, utils.GetSystemPrompt()),
	}

	response, err := s.llm.GenerateContent(context.Background(), prompt, llms.WithTools(registeredTools))
	if err != nil {
		log.Printf("[CallLLM] OpenAI error: %v", err)
		return fmt.Errorf("CallLLM OpenAI API error: %w", err)
	}

	if len(response.Choices) == 0 {
		log.Printf("[CallLLM] Empty response from OpenAI")
		return fmt.Errorf("CallLLM received an empty response from OpenAI")
	}

	var plan []map[string]interface{}
	if err := json.Unmarshal([]byte(response.Choices[0].Content), &plan); err != nil {
		log.Printf("[CallLLM] LLM response is not valid JSON: %v", err)
		return fmt.Errorf("CallLLM returned invalid JSON: %w", err)
	}

	planBytes, err := json.Marshal(plan)
	if err != nil {
		log.Printf("[CallLLM] Failed to marshal plan: %v", err)
		return fmt.Errorf("CallLLM failed to marshal plan: %w", err)
	}

	*reply = string(planBytes)
	log.Printf("[CallLLM] Returning JSON plan: %s", *reply)
	return nil
}

// ExecutePlan processes and executes the plan using the registered tool handlers.
// It returns a full RPCResponse instead of a simple string.
func (s *Server) ExecutePlan(args *string, reply *mcp.RPCResponse) error {
	response := mcp.RPCResponse{
		Version: mcp.JSONRPCVersion,
	}

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

// CallTool allows the client to directly invoke an individual tool.
func (s *Server) CallTool(args *mcp.ToolCallArgs, reply *mcp.RPCResponse) error {
	response := mcp.RPCResponse{
		Version: mcp.JSONRPCVersion,
	}
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

// -----------------------------------------------------------------------------
// Docker Operation Tool Handlers
// -----------------------------------------------------------------------------

func createNetworkHandler(ctx context.Context, s *Server, parameters map[string]interface{}) error {
	name, _ := parameters["name"].(string)
	if name == "" {
		return fmt.Errorf("missing network name")
	}
	_, err := s.dockerClient.NetworkCreate(ctx, name, network.CreateOptions{})
	return err
}

func createContainerHandler(ctx context.Context, s *Server, parameters map[string]interface{}) error {
	name, _ := parameters["name"].(string)
	image, _ := parameters["image"].(string)
	if name == "" || image == "" {
		return errors.New("missing container name or image")
	}
	_, err := s.dockerClient.ContainerCreate(ctx, &container.Config{
		Image: image,
	}, nil, nil, nil, name)
	return err
}

func createVolumeHandler(ctx context.Context, s *Server, parameters map[string]interface{}) error {
	name, _ := parameters["name"].(string)
	if name == "" {
		return fmt.Errorf("invalid or missing name for create_volume action")
	}
	_, err := s.dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	return err
}

func runContainerHandler(ctx context.Context, s *Server, parameters map[string]interface{}) error {
	name, _ := parameters["name"].(string)
	if name == "" {
		return fmt.Errorf("invalid name for run_container")
	}
	return s.dockerClient.ContainerStart(ctx, name, container.StartOptions{})
}

func pullImageHandler(ctx context.Context, s *Server, parameters map[string]interface{}) error {
	// Try to obtain "image" directly.
	image, ok := parameters["image"].(string)
	if !ok || image == "" {
		// Otherwise, attempt to combine "name" and "tag"
		name, nameOk := parameters["name"].(string)
		tag, tagOk := parameters["tag"].(string)
		if !nameOk || name == "" {
			return fmt.Errorf("missing image name for pull_image")
		}
		if !tagOk || tag == "" {
			tag = "latest"
		}
		image = fmt.Sprintf("%s:%s", name, tag)
	}

	// Create a child context with a longer timeout for pulling the image.
	pullCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	out, err := s.dockerClient.ImagePull(pullCtx, image, img.PullOptions{})
	if err != nil {
		return err
	}
	defer out.Close()
	// Consume the output stream to ensure the pull completes.
	_, err = io.Copy(io.Discard, out)
	return err
}

// -----------------------------------------------------------------------------
// JSON-RPC Server Setup and HTTP Adapter
// -----------------------------------------------------------------------------

// StartRPCServer starts an HTTP server on port :1234 that uses net/rpc+JSON-RPC.
func StartRPCServer() {
	srv, err := NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	rpcServer := rpc.NewServer()

	// Register the instance methods under the name "Server"
	if err := rpcServer.RegisterName("Server", srv); err != nil {
		log.Fatalf("Failed to register RPC service: %v", err)
	}

	// Attach an HTTP handler for POST requests to `/rpc`.
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

// httpReadWriteCloser adapts an HTTP request/response into an io.ReadWriteCloser.
type httpReadWriteCloser struct {
	r io.ReadCloser
	w io.Writer
}

func (hrwc *httpReadWriteCloser) Read(p []byte) (int, error) { return hrwc.r.Read(p) }

func (hrwc *httpReadWriteCloser) Write(p []byte) (int, error) { return hrwc.w.Write(p) }

func (hrwc *httpReadWriteCloser) Close() error { return hrwc.r.Close() }

// ----------------------------------------------------------------------------
// main entry point
// ----------------------------------------------------------------------------

func main() {
	// Create a Docker client.
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Error creating Docker client: %v", err)
	}

	// Prepare context and input arguments for prompt generation.
	ctx := context.Background()
	args := map[string]string{
		"name":       "my_project",
		"containers": "[{\"name\": \"example_container\", \"image\": \"nginx:latest\"}]",
	}

	// Generate the system prompt using the GetPrompt function.
	promptResult, err := mcp.GetPrompt(ctx, cli, "docker_compose", args)
	if err != nil {
		log.Fatalf("Error generating system prompt: %v", err)
	}

	// Log the generated prompt messages.
	for _, msg := range promptResult.Messages {
		log.Printf("System Prompt (role=%s):\n%s\n", msg.Role, msg.Content.Text)
	}

	// Start the RPC server.
	// If you want to run the RPC server concurrently, you could launch it as a goroutine.
	// Here, we simply call it directly (it blocks).
	// StartRPCServer()

	go StartRPCServer()
	select {} // Block forever.
}
