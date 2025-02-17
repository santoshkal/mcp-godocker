This file abstracts Docker API operations into helper functions. It provides methods for common Docker operations such as creating networks, containers, volumes, running containers, and pulling images.

## Overview

- CreateNetwork:
  Creates a Docker network using the Docker SDK.

- CreateContainer:
  Creates a Docker container with a specified name and image.

- CreateVolume:
  Creates a Docker volume with the given name.

- RunContainer:
  Starts a container by its name.

- PullImage:
  Pulls a Docker image using parameters from a map. It supports combining separate name and tag keys into a single image reference.

Detailed Explanation:

- CreateNetwork:

```go
func CreateNetwork(ctx context.Context, cli *client.Client, name string) error {
    if name == "" {
        return fmt.Errorf("missing network name")
    }
    _, err := cli.NetworkCreate(ctx, name, network.CreateOptions{})
    return err
}
```

Checks that the network name is provided and calls Docker's `NetworkCreate`.

- CreateContainer:

```go
func CreateContainer(ctx context.Context, cli *client.Client, name, image string) error {
    if name == "" || image == "" {
        return fmt.Errorf("missing container name or image")
    }
    _, err := cli.ContainerCreate(ctx, &container.Config{Image: image}, nil, nil, nil, name)
    return err
}
```

Validates the inputs and creates a container using Docker’s API.

- CreateVolume:

```go
func CreateVolume(ctx context.Context, cli *client.Client, name string) error {
    if name == "" {
        return fmt.Errorf("invalid or missing volume name")
    }
    _, err := cli.VolumeCreate(ctx, volume.CreateOptions{Name: name})
    return err
}
```

Ensures a valid volume name is provided before calling `VolumeCreate`.

- RunContainer:

```go
func RunContainer(ctx context.Context, cli *client.Client, name string) error {
    if name == "" {
        return fmt.Errorf("invalid container name")
    }
    return cli.ContainerStart(ctx, name, container.StartOptions{})
}
```

Starts the container with the specified name.

- PullImage:

```go
func PullImage(ctx context.Context, cli *client.Client, parameters map[string]interface{}) error {
    image, ok := parameters["image"].(string)
    if !ok || image == "" {
        // If image is missing, try to combine name and tag.
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
    // Create a child context with a longer timeout.
    pullCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
    defer cancel()

    out, err := cli.ImagePull(pullCtx, image, img.PullOptions{})
    if err != nil {
        return err
    }
    defer out.Close()
    // Read the stream to ensure pull completes.
    _, err = io.Copy(io.Discard, out)
    return err
}
```

Checks if the "image" key exists; if not, combines "name" and "tag" (defaulting to "latest"). Uses a child context with a 2‑minute timeout to perform the pull.
