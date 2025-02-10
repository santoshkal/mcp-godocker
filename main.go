package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// Server represents the Docker server application.
type Server struct {
	dockerClient *client.Client
	llm          *openai.LLM
}

// Prompt represents a prompt that the server can handle.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Arguments   []PromptArgument `json:"arguments"`
}

// PromptArgument represents an argument for a prompt.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// Tool represents a tool that the server can execute.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
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
func (s *Server) ListPrompts() []Prompt {
	return []Prompt{
		{
			Name:        "docker_compose",
			Description: "Treat the LLM like a Docker Compose manager",
			Arguments: []PromptArgument{
				{
					Name:        "name",
					Description: "Unique name of the project",
					Required:    true,
				},
				{
					Name:        "containers",
					Description: "Describe containers you want",
					Required:    true,
				},
			},
		},
	}
}

// CallLLM sends a user input to the OpenAI LLM and returns the generated plan.
func (s *Server) CallLLM(userInput string) (string, error) {
	sp := `
You are a Docker Compose manager. Generate a plan to: %s

Follow these guidelines:
1. Use the MCP protocol to manage Docker resources.
2. Provide a step-by-step plan in JSON format as an array of actions.
3. Do not pull any image, use existing image tagged as 'latest' available locally.
4. Include only valid Docker actions (e.g., create_container, run_container).

Example response:
[
  {"action": "create_container", "image": "mysql:latest"},
  {"action": "create_container", "image": "wordpress:latest"}
]
`

	prompt := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, sp),
		llms.TextParts(llms.ChatMessageTypeHuman, userInput),
	}
	// Call the OpenAI LLM.
	response, err := s.llm.GenerateContent(context.Background(), prompt, llms.WithJSONMode())
	if err != nil {
		return "", err
	}
	return response.Choices[0].Content, nil
}

// ExecutePlan processes and executes the plan with proper Docker SDK calls.
func (s *Server) ExecutePlan(plan string) (string, error) {
	var response struct {
		Plan []map[string]interface{} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(plan), &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal plan: %v. Plan: %s", err, plan)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute each action in the plan.
	for _, action := range response.Plan {
		switch action["action"] {
		case "create_network":
			name, ok := action["name"].(string)
			if !ok || name == "" {
				return "", fmt.Errorf("invalid or missing name for create_network action")
			}
			_, err := s.dockerClient.NetworkCreate(ctx, name, network.CreateOptions{})
			if err != nil {
				return "", fmt.Errorf("failed to create network %s: %v", name, err)
			}

		case "create_volume":
			name, ok := action["name"].(string)
			if !ok || name == "" {
				return "", fmt.Errorf("invalid or missing name for create_volume action")
			}
			_, err := s.dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: name})
			if err != nil {
				return "", fmt.Errorf("failed to create volume %s: %v", name, err)
			}

		case "create_container":
			name, image, err := parseContainerDetails(action)
			if err != nil {
				return "", err
			}

			envVars, err := parseEnvironment(action)
			if err != nil {
				return "", err
			}

			volumes, err := parseVolumes(action)
			if err != nil {
				return "", err
			}

			networks, err := parseNetworks(action)
			if err != nil {
				return "", err
			}

			config := container.Config{
				Image: image,
				Env:   envVars,
			}
			hostConfig := container.HostConfig{
				Binds: volumes,
			}
			networkingConfig := network.NetworkingConfig{
				EndpointsConfig: make(map[string]*network.EndpointSettings),
			}
			for _, networkName := range networks {
				networkingConfig.EndpointsConfig[networkName] = &network.EndpointSettings{}
			}

			_, err = s.dockerClient.ContainerCreate(ctx, &config, &hostConfig, &networkingConfig, nil, name)
			if err != nil {
				return "", fmt.Errorf("failed to create container %s: %v", name, err)
			}

		case "run_container":
			name, ok := action["name"].(string)
			if !ok {
				return "", fmt.Errorf("invalid name for run_container action: %v", action["name"])
			}

			ctx := context.Background()
			containerJSON, err := s.dockerClient.ContainerInspect(ctx, name)
			if err != nil {
				return "", fmt.Errorf("failed to inspect container %s: %v", name, err)
			}

			if !containerJSON.State.Running {
				if err := s.dockerClient.ContainerStart(ctx, containerJSON.ID, container.StartOptions{}); err != nil {
					return "", fmt.Errorf("failed to start container %s: %v", name, err)
				}
			}

		default:
			return "", fmt.Errorf("unknown action: %s", action["action"])
		}
	}

	fmt.Printf("Devised Plan: %s\n", plan)
	return `{"status": "success", "message": "Plan executed successfully"}`, nil
}

// parseContainerDetails extracts container details from action.
func parseContainerDetails(action map[string]interface{}) (string, string, error) {
	name, ok := action["name"].(string)
	if !ok || name == "" {
		return "", "", fmt.Errorf("invalid or missing container name")
	}
	image, ok := action["image"].(string)
	if !ok || image == "" {
		return "", "", fmt.Errorf("invalid or missing container image")
	}
	return name, image, nil
}

// parseEnvironment converts environment variables to Docker format.
func parseEnvironment(action map[string]interface{}) ([]string, error) {
	envList := []string{}
	if envVars, ok := action["environment"].(map[string]interface{}); ok {
		for key, value := range envVars {
			envList = append(envList, fmt.Sprintf("%s=%v", key, value))
		}
	}
	return envList, nil
}

// parseVolumes extracts and validates volume mappings.
func parseVolumes(action map[string]interface{}) ([]string, error) {
	volumes := []string{}
	if volumeMappings, ok := action["volumes"].([]interface{}); ok {
		for _, volume := range volumeMappings {
			volStr, valid := volume.(string)
			if !valid {
				return nil, fmt.Errorf("invalid volume format: %v", volume)
			}
			volumes = append(volumes, volStr)
		}
	}
	return volumes, nil
}

// parseNetworks extracts and validates network names.
func parseNetworks(action map[string]interface{}) ([]string, error) {
	networks := []string{}
	if networkNames, ok := action["networks"].([]interface{}); ok {
		for _, network := range networkNames {
			netStr, valid := network.(string)
			if !valid {
				return nil, fmt.Errorf("invalid network format: %v", network)
			}
			networks = append(networks, netStr)
		}
	}
	return networks, nil
}

// HTTP Handlers

func (s *Server) handleListPrompts(w http.ResponseWriter, r *http.Request) {
	prompts := s.ListPrompts()
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(toJSON(prompts)))
}

func (s *Server) handleUserInput(w http.ResponseWriter, r *http.Request) {
	userInput := r.URL.Query().Get("input")
	plan, err := s.CallLLM(userInput)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Execute the plan using the MCP protocol.
	response, err := s.ExecutePlan(plan)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(response))
}

// toJSON converts a Go object to a JSON string.
func toJSON(v interface{}) string {
	bytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "%s"}`, err.Error())
	}
	return string(bytes)
}

func main() {
	server, err := NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	http.HandleFunc("/list-prompts", server.handleListPrompts)
	http.HandleFunc("/user-input", server.handleUserInput)

	log.Println("Starting server on :1234...")
	if err := http.ListenAndServe(":1234", nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
