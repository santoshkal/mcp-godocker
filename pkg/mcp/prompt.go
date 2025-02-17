// pkg/mcp/prompt.go

package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

// GetPromptResult represents the result containing one or more prompt messages.
type GetPromptResult struct {
	Messages []PromptMessage `json:"messages"`
}

// PromptMessage is a single message with a role and content.
type PromptMessage struct {
	Role    string      `json:"role"`
	Content TextContent `json:"content"`
}

// TextContent holds the type and text of a prompt message.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// DockerComposePromptInput is the expected input when generating a docker_compose prompt.
type DockerComposePromptInput struct {
	Name       string `json:"name"`
	Containers string `json:"containers"`
}

// GetPrompt generates a system prompt for the given prompt name ("docker_compose")
// using a Docker client to list existing resources. The arguments map should contain
// at least "name" (and optionally "containers").
func GetPrompt(ctx context.Context, cli *client.Client, name string, arguments map[string]string) (GetPromptResult, error) {
	if name != "docker_compose" {
		return GetPromptResult{}, fmt.Errorf("unknown prompt name: %s", name)
	}

	// Validate input arguments.
	input := DockerComposePromptInput{
		Name:       arguments["name"],
		Containers: arguments["containers"],
	}
	if input.Name == "" {
		return GetPromptResult{}, fmt.Errorf("missing required argument 'name'")
	}

	projectLabel := fmt.Sprintf("mcp-server-docker.project=%s", input.Name)

	// List containers with the given label.
	containerFilter := filters.NewArgs()
	containerFilter.Add("label", projectLabel)
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: containerFilter})
	if err != nil {
		return GetPromptResult{}, fmt.Errorf("error listing containers: %w", err)
	}

	// Build container info similar to the Python version.
	containerInfos := make([]map[string]interface{}, 0, len(containers))
	for _, c := range containers {
		var containerName string
		if len(c.Names) > 0 {
			containerName = c.Names[0]
		}
		imageInfo := map[string]interface{}{
			"id":   c.ImageID,
			"tags": []string{c.Image},
		}
		containerInfos = append(containerInfos, map[string]interface{}{
			"name":   containerName,
			"image":  imageInfo,
			"status": c.Status,
			"id":     c.ID,
			"ports":  c.Ports,
		})
	}
	containerJSON, err := json.MarshalIndent(containerInfos, "", "  ")
	if err != nil {
		return GetPromptResult{}, fmt.Errorf("error marshalling container info: %w", err)
	}

	// List volumes with the given label.
	volumeFilter := filters.NewArgs()
	volumeFilter.Add("label", projectLabel)
	volList, err := cli.VolumeList(ctx, volume.ListOptions{Filters: volumeFilter})
	if err != nil {
		return GetPromptResult{}, fmt.Errorf("error listing volumes: %w", err)
	}
	volumeInfos := make([]map[string]interface{}, 0, len(volList.Volumes))
	for _, v := range volList.Volumes {
		volumeInfos = append(volumeInfos, map[string]interface{}{
			"name": v.Name,
			"id":   v.Name, // Using the volume name as its identifier.
		})
	}
	volumesJSON, err := json.MarshalIndent(volumeInfos, "", "  ")
	if err != nil {
		return GetPromptResult{}, fmt.Errorf("error marshalling volume info: %w", err)
	}

	// List networks with the given label.
	networkFilter := filters.NewArgs()
	networkFilter.Add("label", projectLabel)
	networks, err := cli.NetworkList(ctx, network.ListOptions{Filters: networkFilter})
	if err != nil {
		return GetPromptResult{}, fmt.Errorf("error listing networks: %w", err)
	}
	networkInfos := make([]map[string]interface{}, 0, len(networks))
	for _, n := range networks {
		containerList := []map[string]interface{}{}
		// n.Containers is a map from container ID to network.EndpointResource.
		for containerID := range n.Containers {
			containerList = append(containerList, map[string]interface{}{
				"id": containerID,
			})
		}
		networkInfos = append(networkInfos, map[string]interface{}{
			"name":       n.Name,
			"id":         n.ID,
			"containers": containerList,
		})
	}
	networksJSON, err := json.MarshalIndent(networkInfos, "", "  ")
	if err != nil {
		return GetPromptResult{}, fmt.Errorf("error marshalling network info: %w", err)
	}

	// Build the multi-line prompt text.
	// (Note: single quotes are used instead of backticks in some places to avoid syntax issues.)
	text := fmt.Sprintf(`
You are going to act as a Docker Compose manager, using the Docker Tools
available to you. Instead of being provided a 'docker-compose.yml' file,
you will be given instructions in plain language, and interact with the
user through a plan+apply loop, akin to how Terraform operates.

Every Docker resource you create must be assigned the following label:

    %s

You should use this label to filter resources when possible.

Every Docker resource you create must also be prefixed with the project name, followed by a dash ('-'):

    %s-{ResourceName}

Here are the resources currently present in the project, based on the presence of the above label:

<BEGIN CONTAINERS>
%s
<END CONTAINERS>
<BEGIN VOLUMES>
%s
<END VOLUMES>
<BEGIN NETWORKS>
%s
<END NETWORKS>

Do not retry the same failed action more than once. Prefer terminating your output
when presented with 3 errors in a row, and ask a clarifying question to
form better inputs or address the error.

For container images, always prefer using the 'latest' image tag, unless the user specifies a tag specifically.
So if a user asks to deploy Nginx, you should pull 'nginx:latest'.

Below is a description of the state of the Docker resources which the user would like you to manage:

<BEGIN DOCKER-RESOURCES>
%s
<END DOCKER-RESOURCES>

Respond to this message with a plan of what you will do, in the EXACT format below:

<BEGIN FORMAT>
## Introduction

I will be assisting with deploying Docker containers for project: '%s'.

### Plan+Apply Loop

I will run in a plan+apply loop when you request changes to the project. This is
to ensure that you are aware of the changes I am about to make, and to give you
the opportunity to ask questions or make tweaks.

Instruct me to apply immediately (without confirming the plan with you) when you desire to do so.

## Commands

Instruct me with the following commands at any point:

- 'help': print this list of commands
- 'apply': apply a given plan
- 'down': stop containers in the project
- 'ps': list containers in the project
- 'quiet': turn on quiet mode (default)
- 'verbose': turn on verbose mode (I will explain a lot!)
- 'destroy': produce a plan to destroy all resources in the project

## Plan

I plan to take the following actions:

1. CREATE ...
2. READ ...
3. UPDATE ...
4. DESTROY ...
5. RECREATE ...
...
N. ...

Respond 'apply' to apply this plan. Otherwise, provide feedback and I will present you with an updated plan.
<END FORMAT>

Always apply a plan in dependency order. For example, if you are creating a container that depends on a
database, create the database first, and abort the apply if dependency creation fails. Likewise, 
destruction should occur in the reverse dependency order, and be aborted if destroying a particular resource fails.

Plans should only create, update, or destroy resources in the project. Relatedly, 'recreate' should
be used to indicate a destroy followed by a create; always prefer updating a resource when possible,
only recreating it if required (e.g. for immutable resources like containers).
`, projectLabel, input.Name, string(containerJSON), string(volumesJSON), string(networksJSON), input.Containers, input.Name)

	// Create a prompt message with role "user" and the generated text.
	message := PromptMessage{
		Role: "user",
		Content: TextContent{
			Type: "text",
			Text: text,
		},
	}

	return GetPromptResult{
		Messages: []PromptMessage{message},
	}, nil
}
