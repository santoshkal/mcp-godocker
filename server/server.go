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
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"

	"santoshkal/mcp-godocker/types" // Shared package with Prompt definitions, etc.
)

// Server represents the Docker server application.
type Server struct {
	dockerClient *client.Client
	llm          *openai.LLM
}

// NewServer creates a new Server instance.
func NewServer() (*Server, error) {
	// Initialize Docker client.
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	// Initialize OpenAI LLM.
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}
	llm, err := openai.New(openai.WithToken(apiKey), openai.WithModel("gpt-4o"))
	if err != nil {
		return nil, err
	}

	return &Server{dockerClient: dockerClient, llm: llm}, nil
}

// ListPrompts returns a list of available prompts.
// net/rpc expects the method signature to be:
//
//	func (s *Server) SomeMethod(args *SomeType, reply *SomeType) error
//
// Here, we ignore `args` (it’s just a placeholder struct{}).
func (s *Server) ListPrompts(args *struct{}, reply *[]types.Prompt) error {
	*reply = []types.Prompt{
		{
			Name:        "docker_compose",
			Description: "Treat the LLM like a Docker Compose manager",
			Arguments: []types.PromptArgument{
				{Name: "name", Description: "Unique name of the project", Required: true},
				{Name: "containers", Description: "Describe containers you want", Required: true},
			},
		},
	}
	return nil
}

// CallLLM sends a user input to the OpenAI LLM and returns the generated plan.
// The client would call it with one string argument and get one string reply:
//
//	callRPC("Server.CallLLM", "Your input to LLM")
func (s *Server) CallLLM(args *string, reply *string) error {
	log.Printf("[CallLLM] Received user input: %s", *args)

	// Define strict JSON format for LLM response
	promptTemplate := `
You are an AI that generates structured JSON plans for Docker automation.
Always return a valid JSON array of actions.
	Follow these guidelines:
1. Use the MCP protocol to manage Docker resources.
2. Provide a step-by-step plan in JSON format as an array of actions.
3. Do not pull any image, use existing image tagged as 'latest' available locally.
4. Include only valid Docker actions (e.g., create_container, run_container).

---
Example Response:
[
    {
        "action": "create_network",
        "parameters": {
            "name": "mysql_network",
            "driver": "bridge"
        }
    },
    {
        "action": "create_volume",
        "parameters": {
            "name": "mysql_data"
        }
    },
    {
        "action": "create_container",
        "parameters": {
            "name": "mysql_container",
            "image": "mysql:latest",
            "environment": {
                "MYSQL_ROOT_PASSWORD": "rootpassword",
                "MYSQL_DATABASE": "exampledb",
                "MYSQL_USER": "exampleuser",
                "MYSQL_PASSWORD": "examplepass"
            },
            "volumes": [
                {
                    "source": "mysql_data",
                    "target": "/var/lib/mysql"
                }
            ],
            "networks": [
                "mysql_network"
            ],
            "ports": [
                {
                    "published": 3306,
                    "target": 3306
                }
            ]
        }
    },
    {
        "action": "run_container",
        "parameters": {
            "name": "mysql_container"
        }
    }
]
---
Do not include explanations. Do not return Markdown. Just return JSON.
`
	prompt := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, fmt.Sprintf(promptTemplate, *args)),
		llms.TextParts(llms.ChatMessageTypeHuman, *args),
	}

	response, err := s.llm.GenerateContent(context.Background(), prompt)
	if err != nil {
		log.Printf("[CallLLM] OpenAI error: %v", err)
		return fmt.Errorf("CallLLM OpenAI API error: %w", err)
	}

	if len(response.Choices) == 0 {
		log.Printf("[CallLLM] Empty response from OpenAI")
		return fmt.Errorf("CallLLM received an empty response from OpenAI")
	}

	// Ensure response is valid JSON
	var plan []map[string]interface{}
	if err := json.Unmarshal([]byte(response.Choices[0].Content), &plan); err != nil {
		log.Printf("[CallLLM] LLM response is not valid JSON: %v", err)
		return fmt.Errorf("CallLLM returned invalid JSON: %w", err)
	}

	// Convert structured response back to JSON string
	planBytes, err := json.Marshal(plan)
	if err != nil {
		log.Printf("[CallLLM] Failed to marshal plan: %v", err)
		return fmt.Errorf("CallLLM failed to marshal plan: %w", err)
	}

	*reply = string(planBytes)
	log.Printf("[CallLLM] Returning JSON plan: %s", *reply)
	return nil
}

// ExecutePlan processes and executes the plan with proper Docker SDK calls.
// The client would call it with one string argument (the plan in JSON), and get a string reply:
func (s *Server) ExecutePlan(args *string, reply *string) error {
	if args == nil || *args == "" {
		return fmt.Errorf("ExecutePlan received empty plan")
	}

	log.Printf("[ExecutePlan] Received Plan: %s", *args)

	var plan []map[string]interface{}
	if err := json.Unmarshal([]byte(*args), &plan); err != nil {
		log.Printf("[ExecutePlan] Error unmarshalling JSON: %v", err)
		log.Printf("[ExecutePlan] Raw Plan Received: %s", *args)
		return fmt.Errorf("failed to parse plan JSON: %w", err)
	}

	if len(plan) == 0 {
		log.Printf("[ExecutePlan] No actions found in plan")
		return fmt.Errorf("received empty plan from LLM")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, action := range plan {
		log.Printf("[ExecutePlan] Processing action: %+v", action)

		actionType, ok := action["action"].(string)
		if !ok || actionType == "" {
			log.Printf("[ExecutePlan] Invalid action format: %+v", action)
			return fmt.Errorf("invalid action format")
		}

		parameters, _ := action["parameters"].(map[string]interface{})

		switch actionType {
		case "create_network":
			name, _ := parameters["name"].(string)
			if name == "" {
				return errors.New("missing network name")
			}
			_, err := s.dockerClient.NetworkCreate(ctx, name, network.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create network %s: %v", name, err)
			}

		case "create_container":
			name, _ := parameters["name"].(string)
			image, _ := parameters["image"].(string)
			if name == "" || image == "" {
				return errors.New("missing container name or image")
			}

			_, err := s.dockerClient.ContainerCreate(ctx, &container.Config{
				Image: image,
			}, nil, nil, nil, name)
			if err != nil {
				return fmt.Errorf("failed to create container %s: %v", name, err)
			}
		case "create_volume":
			name, _ := parameters["name"].(string)
			if name == "" {
				return fmt.Errorf("invalid or missing name for create_volume action")
			}
			_, err := s.dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: name})
			if err != nil {
				return fmt.Errorf("failed to create volume %s: %v", name, err)
			}
		case "run_container":
			name, _ := parameters["name"].(string)
			if name == "" {
				return fmt.Errorf("invalid name for run_container")
			}
			if err := s.dockerClient.ContainerStart(ctx, name, container.StartOptions{}); err != nil {
				return fmt.Errorf("failed to start container %s: %v", name, err)
			}

		default:
			return fmt.Errorf("unknown action: %s", actionType)
		}
	}

	*reply = `{"status": "success", "message": "Plan executed successfully"}`
	return nil
}

// Helper functions (unchanged) ----------------------------------------------

func parseContainerDetails(action map[string]interface{}) (string, string, error) {
	name, ok := action["name"].(string)
	if !ok || name == "" {
		return "", "", errors.New("invalid or missing container name")
	}
	image, ok := action["image"].(string)
	if !ok || image == "" {
		return "", "", errors.New("invalid or missing container image")
	}
	return name, image, nil
}

func parseEnvironment(action map[string]interface{}) ([]string, error) {
	var envList []string
	if envVars, ok := action["environment"].(map[string]interface{}); ok {
		for key, value := range envVars {
			envList = append(envList, fmt.Sprintf("%s=%v", key, value))
		}
	}
	return envList, nil
}

func parseVolumes(action map[string]interface{}) ([]string, error) {
	var volumes []string
	if volumeMappings, ok := action["volumes"].([]interface{}); ok {
		for _, vol := range volumeMappings {
			volStr, valid := vol.(string)
			if !valid {
				return nil, fmt.Errorf("invalid volume format: %v", vol)
			}
			volumes = append(volumes, volStr)
		}
	}
	return volumes, nil
}

func parseNetworks(action map[string]interface{}) ([]string, error) {
	var networks []string
	if networkNames, ok := action["networks"].([]interface{}); ok {
		for _, nw := range networkNames {
			netStr, valid := nw.(string)
			if !valid {
				return nil, fmt.Errorf("invalid network format: %v", nw)
			}
			networks = append(networks, netStr)
		}
	}
	return networks, nil
}

// StartRPCServer starts an HTTP server on port :1234 that uses
// net/rpc + JSON-RPC to serve methods of our Server type.
func StartRPCServer() {
	srv, err := NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	rpcServer := rpc.NewServer()

	// Register the instance methods under the name "Server"
	// so methods will be called as "Server.ListPrompts", "Server.CallLLM", etc.
	if err := rpcServer.RegisterName("Server", srv); err != nil {
		log.Fatalf("Failed to register RPC service: %v", err)
	}

	// Attach an HTTP handler for POST requests to `/rpc`.
	http.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "JSON-RPC requires POST", http.StatusMethodNotAllowed)
			return
		}
		// The net/rpc library uses an io.ReadWriteCloser for the codec.
		// We wrap the request body (for reading) + response writer (for writing).
		rpcServer.ServeCodec(jsonrpc.NewServerCodec(&httpReadWriteCloser{
			r: r.Body,
			w: w,
		}))
	})

	log.Println("JSON-RPC server listening on port 1234 (POST /rpc)...")
	log.Fatal(http.ListenAndServe(":1234", nil))
}

// httpReadWriteCloser is a simple shim to adapt an http request/response
// into an io.ReadWriteCloser for net/rpc/jsonrpc.
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
	StartRPCServer()

	// Keep the main goroutine alive.
	select {}
}
