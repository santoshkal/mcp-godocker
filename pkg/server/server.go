package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"

	"santoshkal.com/mcp-godocker/pkg/mcp"
	"santoshkal.com/mcp-godocker/pkg/reg"
	"santoshkal.com/mcp-godocker/pkg/utils"
)

// Create a logger instance using logrus.
var logger = logrus.New()

func init() {
	// Configure the logger for full timestamps and set debug level.
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logger.SetLevel(logrus.DebugLevel)
}

// RegisteredTool holds metadata and the handler for a tool.
type RegisteredTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	Handler     mcp.ToolHandler
	ServiceName string
}

// Service defines an interface for a service to register its tools.
type Service interface {
	Name() string
	RegisterTools(s *Server)
}

// Server represents the composite server and implements mcp.Registry.
type Server struct {
	dockerClient *client.Client
	llm          *openai.LLM
	tools        map[string]RegisteredTool
	services     map[string]Service
}

// Ensure Server implements mcp.Registry.
var _ mcp.Registry = (*Server)(nil)

// NewServer initializes a new Server instance and registers dynamic tools.
func NewServer() (*Server, error) {
	logger.Debug("Entering NewServer")
	defer logger.Debug("Exiting NewServer")

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logger.Errorf("Docker client initialization failed: %v", err)
		dockerClient = nil // Allow server initialization without Docker.
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
		services:     make(map[string]Service),
	}

	// Dynamically load and register tools from YAML configuration.
	homeDir := os.Getenv("HOME")
	configPath := homeDir + "/mcp-godocker/config.yaml"
	logger.Infof("Config Path: %v", configPath)
	if configPath == "" {
		configPath = "config.yaml" // Default configuration file.
	}
	if err := reg.RegisterToolsFromConfig(s, configPath); err != nil {
		logger.Errorf("failed to register dynamic tools from config: %v", err)
		// Optionally, you can return the error if dynamic tools are critical.
	}

	return s, nil
}

// RegisterTool implements the mcp.Registry interface.
func (s *Server) RegisterTool(name, description string, inputSchema map[string]interface{}, handler mcp.ToolHandler) {
	logger.Debugf("Registering tool: %s", name)
	s.tools[name] = RegisteredTool{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
		Handler:     handler,
	}
}

// RegisterService registers a service and lets it add its tools.
func (s *Server) RegisterService(service Service) {
	logger.Debugf("Registering service: %s", service.Name())
	s.services[service.Name()] = service
	service.RegisterTools(s)
}

// listTools returns a slice of strings listing all registered tools.
func (s *Server) listTools() []string {
	var toolList []string
	for _, tool := range s.tools {
		toolList = append(toolList, fmt.Sprintf("%s: %s", tool.Name, tool.Description))
	}
	return toolList
}

// listServices returns a slice of strings listing all registered services.
func (s *Server) listServices() []string {
	var serviceList []string
	for _, svc := range s.services {
		serviceList = append(serviceList, svc.Name())
	}
	return serviceList
}

// listToolsForService returns tools filtered by service name.
func (s *Server) listToolsForService(serviceName string) []string {
	var toolList []string
	for _, tool := range s.tools {
		if strings.ToLower(tool.ServiceName) == strings.ToLower(serviceName) {
			toolList = append(toolList, fmt.Sprintf("%s: %s", tool.Name, tool.Description))
		}
	}
	return toolList
}

// ProcessInstruction handles a plain language instruction.
func (s *Server) ProcessInstruction(instruction *string, reply *mcp.RPCResponse) error {
	logger.Debugf("Entering ProcessInstruction with instruction: %s", *instruction)
	defer logger.Debug("Exiting ProcessInstruction")

	lowerInst := strings.ToLower(*instruction)

	// If the query asks for services, return the list.
	if utils.IsListServicesQuery(lowerInst) {
		services := s.listServices()
		res, err := json.Marshal(services)
		if err != nil {
			return fmt.Errorf("failed to marshal services list: %w", err)
		}
		*reply = mcp.RPCResponse{Version: mcp.JSONRPCVersion, Result: json.RawMessage(res)}
		return nil
	}

	// If the query asks for tools, optionally extract a service name.
	if utils.IsListToolsQuery(lowerInst) {
		if serviceName, found := utils.ExtractServiceName(lowerInst); found {
			tools := s.listToolsForService(serviceName)
			res, err := json.Marshal(tools)
			if err != nil {
				return fmt.Errorf("failed to marshal tools list: %w", err)
			}
			*reply = mcp.RPCResponse{Version: mcp.JSONRPCVersion, Result: json.RawMessage(res)}
			return nil
		}
		tools := s.listTools()
		res, err := json.Marshal(tools)
		if err != nil {
			return fmt.Errorf("failed to marshal tools list: %w", err)
		}
		*reply = mcp.RPCResponse{Version: mcp.JSONRPCVersion, Result: json.RawMessage(res)}
		return nil
	}

	universalPrompt := utils.GetSystemPrompt()
	logger.Debug("[ProcessInstruction] Using universal system prompt.")
	utils.SetSystemPromptOverride(universalPrompt)
	defer utils.ClearSystemPromptOverride()

	var plan string
	if err := s.CallLLM(instruction, &plan); err != nil {
		return fmt.Errorf("ProcessInstruction: failed to call LLM: %w", err)
	}

	logger.Debugf("[ProcessInstruction] Generated plan: %s", plan)
	if err := s.ExecutePlan(&plan, reply); err != nil {
		return fmt.Errorf("ProcessInstruction: failed to execute plan: %w", err)
	}

	return nil
}

// DockerClient returns the Docker client instance.
func (s *Server) DockerClient() *client.Client {
	return s.dockerClient
}

// invokeTool executes a tool based on the LLM function call.
func (s *Server) invokeTool(functionCall *llms.FunctionCall) (string, error) {
	logger.Debugf("Entering invokeTool for function: %s", functionCall.Name)
	defer logger.Debug("Exiting invokeTool")

	tool, exists := s.tools[functionCall.Name]
	if !exists {
		return "", fmt.Errorf("Tool %s not found", functionCall.Name)
	}

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(functionCall.Arguments), &params); err != nil {
		return "", fmt.Errorf("invalid arguments for tool %s: %v", functionCall.Name, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := tool.Handler(ctx, s, params); err != nil {
		return "", fmt.Errorf("error executing tool %s: %v", functionCall.Name, err)
	}

	return fmt.Sprintf("Tool %s executed successfully", functionCall.Name), nil
}

// CallLLM sends input to the LLM and returns a generated JSON plan.
func (s *Server) CallLLM(input *string, reply *string) error {
	logger.Debugf("Entering CallLLM with input: %s", *input)
	defer logger.Debug("Exiting CallLLM")

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
		llms.TextParts(llms.ChatMessageTypeHuman, *input),
		llms.TextParts(llms.ChatMessageTypeSystem, utils.GetSystemPrompt()),
	}

	// Uncomment if you wish to log the full prompt.
	// logger.Debugf("[Combined Prompt] Prompt sent to LLM: %v", prompt)

	response, err := s.llm.GenerateContent(
		context.Background(),
		prompt,
		llms.WithTools(registeredTools),
		llms.WithJSONMode(),
	)
	if err != nil {
		logger.Errorf("[CallLLM] OpenAI error: %v", err)
		return fmt.Errorf("LLM API error: %w", err)
	}

	if len(response.Choices) == 0 {
		logger.Errorf("[CallLLM] Empty response from LLM")
		return fmt.Errorf("LLM returned an empty response")
	}

	rawContent := response.Choices[0].Content
	logger.Debugf("[CallLLM] Raw LLM response: %q", rawContent)

	toolCalls := response.Choices[0].ToolCalls
	if len(toolCalls) > 0 {
		for _, toolCall := range toolCalls {
			if toolCall.FunctionCall != nil {
				logger.Debugf("[Tool Invoked] Function: %s, Arguments: %s", toolCall.FunctionCall.Name, toolCall.FunctionCall.Arguments)
				if result, err := s.invokeTool(toolCall.FunctionCall); err == nil {
					*reply = result
					return nil
				} else {
					logger.Errorf("[CallLLM] Tool invocation error: %v", err)
				}
			}
		}
	} else {
		*reply = rawContent
	}

	return nil
}

// ExecutePlan processes the JSON plan generated by the LLM.
func (s *Server) ExecutePlan(planJSON *string, reply *mcp.RPCResponse) error {
	logger.Debugf("Entering ExecutePlan with plan: %s", *planJSON)
	defer logger.Debug("Exiting ExecutePlan")

	response := mcp.RPCResponse{Version: mcp.JSONRPCVersion}
	if planJSON == nil || *planJSON == "" {
		response.Error = mcp.NewError(-32602, "ExecutePlan received empty plan")
		*reply = response
		return nil
	}

	logger.Debugf("[ExecutePlan] Received Plan: %s", *planJSON)

	var raw interface{}
	if err := json.Unmarshal([]byte(*planJSON), &raw); err != nil {
		logger.Errorf("[ExecutePlan] Error unmarshalling JSON: %v", err)
		response.Error = mcp.NewError(-32700, fmt.Sprintf("failed to parse plan JSON: %v", err))
		*reply = response
		return nil
	}

	var plan []map[string]interface{}
	switch v := raw.(type) {
	case []interface{}:
		for _, elem := range v {
			if m, ok := elem.(map[string]interface{}); ok {
				plan = append(plan, m)
			} else {
				response.Error = mcp.NewError(-32700, "plan array contains non-object element")
				*reply = response
				return nil
			}
		}
	case map[string]interface{}:
		plan = []map[string]interface{}{v}
	default:
		response.Error = mcp.NewError(-32700, "plan JSON is neither an object nor an array")
		*reply = response
		return nil
	}

	if len(plan) == 0 {
		logger.Errorf("[ExecutePlan] No actions found in plan")
		response.Error = mcp.NewError(-32602, "received empty plan from LLM")
		*reply = response
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, action := range plan {
		logger.Debugf("[ExecutePlan] Processing action: %+v", action)
		actionType, ok := action["action"].(string)
		if !ok || actionType == "" {
			response.Error = mcp.NewError(-32602, "invalid action format")
			*reply = response
			return nil
		}
		if strings.HasPrefix(actionType, "functions.") {
			actionType = strings.TrimPrefix(actionType, "functions.")
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
	logger.Debugf("Entering CallTool for tool: %s", args.ToolName)
	defer logger.Debug("Exiting CallTool")

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
	logger.Infof("Starting JSON-RPC server on port 1234 (POST /rpc)...")
	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("Server", s); err != nil {
		logger.Fatalf("Failed to register RPC service: %v", err)
	}

	http.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "JSON-RPC requires POST", http.StatusMethodNotAllowed)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request", http.StatusBadRequest)
			return
		}
		var req mcp.RPCRequest
		if err := json.Unmarshal(data, &req); err == nil && req.Method != "" {
			codec := jsonrpc.NewServerCodec(&httpReadWriteCloser{
				r: io.NopCloser(bytes.NewBuffer(data)),
				w: w,
			})
			rpcServer.ServeRequest(codec)
		} else {
			instruction := string(data)
			logger.Debugf("Received plain text instruction: %s", instruction)
			var reply mcp.RPCResponse
			if err := s.ProcessInstruction(&instruction, &reply); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			result, err := json.Marshal(reply)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(result)
		}
	})

	logger.Infof("JSON-RPC server listening on port 1234 (POST /rpc)...")
	logger.Fatal(http.ListenAndServe(":1234", nil))
}
