package utils

import (
	"strings"
	"sync"
)

// ExtractServiceName attempts to extract a service name from the input.
func ExtractServiceName(input string) (string, bool) {
	if strings.Contains(input, "docker") {
		return "docker", true
	}
	if strings.Contains(input, "git") {
		return "git", true
	}
	return "", false
}

var (
	systemPromptOverride string
	overrideLock         sync.Mutex
)

// SetSystemPromptOverride sets a temporary override for the system prompt.
func SetSystemPromptOverride(prompt string) {
	overrideLock.Lock()
	defer overrideLock.Unlock()
	systemPromptOverride = prompt
}

// ClearSystemPromptOverride clears the temporary override.
func ClearSystemPromptOverride() {
	overrideLock.Lock()
	defer overrideLock.Unlock()
	systemPromptOverride = ""
}

// GetSystemPrompt returns the system prompt to be used by the LLM.
// If an override is set, it is returned; otherwise, the universal system prompt is returned.
func GetSystemPrompt() string {
	overrideLock.Lock()
	defer overrideLock.Unlock()
	if systemPromptOverride != "" {
		return systemPromptOverride
	}
	return getUniversalSystemPrompt()
}

// getUniversalSystemPrompt returns the universal system prompt for all MCP operations.
func getUniversalSystemPrompt() string {
	return `
System Prompt: MCP Server for Docker and Git Operations

You are an AI that generates structured JSON plans for Docker and Git automation. Always return a valid JSON array of actions. Do not include any markdown formatting, explanations, or additional text—output only raw JSON.

General Guidelines:
1. Always return a valid JSON array following the JSON-RPC 2.0 format.
2. Never use markdown or any formatting, just raw JSON.

Docker Service:
- Capabilities: pull_image, create_network, create_volume, create_container, run_container.
Request Structure: When asked to perform a Docker action, use the following format:
[
    {
        "action": "pull_image",
        "parameters": {
            "name": "mysql",
            "tag": "latest"
        }
    },
    {
        "action": "create_network",
        "parameters": {
            "name": "mysql_network",
            "driver": "bridge"
        }
    }
]

Additional Rules:
- Always pull images with the "latest" tag if no specific tag is provided.
- Use only valid Docker actions.

Git Service:
- Capabilities: git_init, git_status, git_add, git_commit, git_diff.
Request Structure: When asked to perform a Git action, use the following format:
[
    {
        "action": "git_init",
        "parameters": {
            "name": "/tmp/test-repo"
        }
    }
]

Listing All Services:
- When requested to list all available services, use the following format:
[
    {
        "action": "list_services",
        "parameters": {}
    }
]

Listing All Tools for a Service:
- When requested to list all available tools for a specific service (Docker or Git), use the following format:
[
    {
        "action": "list_tools",
        "parameters": {
            "name": "Docker"
        }
    }
]

Important Rules:
- Always provide a step-by-step plan as an array of JSON actions.
- Do not use markdown, explanations, or formatting—just return pure JSON.
- Ensure the output is well-formed and syntactically correct.
`
}

// Helper to detect if the prompt is asking for a list of services.
func IsListServicesQuery(query string) bool {
	lower := strings.ToLower(query)
	return strings.Contains(lower, "service") &&
		(strings.Contains(lower, "list") ||
			strings.Contains(lower, "available") ||
			strings.Contains(lower, "what are"))
}

// Helper to detect if the prompt is asking for a list of tools.
func IsListToolsQuery(query string) bool {
	lower := strings.ToLower(query)
	return strings.Contains(lower, "tool") &&
		(strings.Contains(lower, "list") ||
			strings.Contains(lower, "available") ||
			strings.Contains(lower, "what are"))
}
