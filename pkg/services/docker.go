// pkg/services/docker.go

package services

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	img "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"

	"santoshkal/mcp-godocker/pkg/server"
)

type DockerService struct{}

func NewDockerService() (*DockerService, error) {
	// Additional Docker-specific initialization can be added here.
	return &DockerService{}, nil
}

func (ds *DockerService) Name() string {
	return "docker"
}

func (ds *DockerService) RegisterTools(s *server.Server) {
	s.RegisterTool("create_network", "Create a Docker network", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the network",
			},
		},
		"required": []string{"name"},
	}, createNetworkHandler(s))

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
	}, createContainerHandler(s))

	s.RegisterTool("create_volume", "Create a Docker volume", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the volume",
			},
		},
		"required": []string{"name"},
	}, createVolumeHandler(s))

	s.RegisterTool("run_container", "Run (start) a Docker container", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the container",
			},
		},
		"required": []string{"name"},
	}, runContainerHandler(s))

	s.RegisterTool("pull_image", "Pull a Docker image", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"image": map[string]interface{}{
				"type":        "string",
				"description": "Name of the image to pull",
			},
		},
		"required": []string{"image"},
	}, pullImageHandler(s))
}

func createNetworkHandler(s *server.Server) server.ToolHandler {
	return func(ctx context.Context, srv *server.Server, parameters map[string]interface{}) error {
		name, _ := parameters["name"].(string)
		if name == "" {
			return fmt.Errorf("missing network name")
		}
		_, err := srv.DockerClient().NetworkCreate(ctx, name, network.CreateOptions{})
		return err
	}
}

func createContainerHandler(s *server.Server) server.ToolHandler {
	return func(ctx context.Context, srv *server.Server, parameters map[string]interface{}) error {
		name, _ := parameters["name"].(string)
		image, _ := parameters["image"].(string)
		if name == "" || image == "" {
			return fmt.Errorf("missing container name or image")
		}
		_, err := srv.DockerClient().ContainerCreate(ctx, &container.Config{
			Image: image,
		}, nil, nil, nil, name)
		return err
	}
}

func createVolumeHandler(s *server.Server) server.ToolHandler {
	return func(ctx context.Context, srv *server.Server, parameters map[string]interface{}) error {
		name, _ := parameters["name"].(string)
		if name == "" {
			return fmt.Errorf("invalid or missing name for create_volume action")
		}
		_, err := srv.DockerClient().VolumeCreate(ctx, volume.CreateOptions{Name: name})
		return err
	}
}

func runContainerHandler(s *server.Server) server.ToolHandler {
	return func(ctx context.Context, srv *server.Server, parameters map[string]interface{}) error {
		name, _ := parameters["name"].(string)
		if name == "" {
			return fmt.Errorf("invalid name for run_container")
		}
		return srv.DockerClient().ContainerStart(ctx, name, container.StartOptions{})
	}
}

func pullImageHandler(s *server.Server) server.ToolHandler {
	return func(ctx context.Context, srv *server.Server, parameters map[string]interface{}) error {
		image, ok := parameters["image"].(string)
		if !ok || image == "" {
			return fmt.Errorf("missing image name for pull_image")
		}
		pullCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		out, err := srv.DockerClient().ImagePull(pullCtx, image, img.PullOptions{})
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(io.Discard, out)
		return err
	}
}
