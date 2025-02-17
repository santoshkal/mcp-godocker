package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	img "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

// CreateNetwork creates a Docker network with the given name.
func CreateNetwork(ctx context.Context, cli *client.Client, name string) error {
	if name == "" {
		return fmt.Errorf("missing network name")
	}
	_, err := cli.NetworkCreate(ctx, name, network.CreateOptions{})
	return err
}

// CreateContainer creates a Docker container with the given name and image.
func CreateContainer(ctx context.Context, cli *client.Client, name, image string) error {
	if name == "" || image == "" {
		return fmt.Errorf("missing container name or image")
	}
	_, err := cli.ContainerCreate(ctx, &container.Config{Image: image}, nil, nil, nil, name)
	return err
}

// CreateVolume creates a Docker volume with the given name.
func CreateVolume(ctx context.Context, cli *client.Client, name string) error {
	if name == "" {
		return fmt.Errorf("invalid or missing volume name")
	}
	_, err := cli.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	return err
}

// RunContainer starts the Docker container with the given name.
func RunContainer(ctx context.Context, cli *client.Client, name string) error {
	if name == "" {
		return fmt.Errorf("invalid container name")
	}
	return cli.ContainerStart(ctx, name, container.StartOptions{})
}

// PullImage pulls a Docker image. It accepts a parameters map so that if the image name is not directly provided,
// it will combine "name" and "tag" (defaulting tag to "latest").
func PullImage(ctx context.Context, cli *client.Client, parameters map[string]interface{}) error {
	image, ok := parameters["image"].(string)
	if !ok || image == "" {
		// Try to combine "name" and "tag"
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
	// Use a child context with a longer timeout for image pulling.
	pullCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	out, err := cli.ImagePull(pullCtx, image, img.PullOptions{})
	if err != nil {
		return err
	}
	defer out.Close()
	// Consume the output stream so the pull completes.
	_, err = io.Copy(io.Discard, out)
	return err
}
