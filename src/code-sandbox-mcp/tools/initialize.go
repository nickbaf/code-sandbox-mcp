package tools

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerImage "github.com/docker/docker/api/types/image"
	"github.com/mark3labs/mcp-go/mcp"
)

// InitializeEnvironment creates a new container for code execution
func InitializeEnvironment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get the requested Docker image or use default
	image, ok := request.Params.Arguments["image"].(string)
	if !ok || image == "" {
		// Default to a slim debian image with Python pre-installed
		image = "python:3.12-slim-bookworm"
	}

	// Create and start the container
	containerId, err := createContainer(ctx, image)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("container_id: %s", containerId)), nil
}

// createContainer creates a new Docker container and returns its ID
func createContainer(ctx context.Context, image string) (string, error) {
	// Try to create Docker client with multiple fallback options
	cli, err := createDockerClient()
	if err != nil {
		return "", fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	// Check if image exists locally first
	_, err = cli.ImageInspect(ctx, image)
	if err != nil {
		// Image doesn't exist locally, so pull it
		fmt.Printf("Docker image %s not found locally, pulling from registry...\n", image)

		// Create a context with timeout for the pull operation
		// This prevents hanging when the image doesn't exist in the registry
		pullCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		pullReader, pullErr := cli.ImagePull(pullCtx, image, dockerImage.PullOptions{})
		if pullErr != nil {
			// Check if this is a timeout error
			if pullCtx.Err() == context.DeadlineExceeded {
				return "", fmt.Errorf("timeout while trying to pull Docker image %s - this usually means the image doesn't exist in the registry or the registry is unreachable", image)
			}
			// Check for common "not found" error patterns
			errStr := pullErr.Error()
			if strings.Contains(errStr, "not found") || strings.Contains(errStr, "404") || strings.Contains(errStr, "manifest unknown") {
				return "", fmt.Errorf("docker image %s not found in registry. Please check that the image name and tag are correct", image)
			}
			return "", fmt.Errorf("failed to pull Docker image %s: %w", image, pullErr)
		}
		defer pullReader.Close()

		// Read the pull response to ensure it completes
		// This also provides feedback on the pull progress
		pullOutput, readErr := io.ReadAll(pullReader)
		if readErr != nil {
			// Check if this is a timeout error
			if pullCtx.Err() == context.DeadlineExceeded {
				return "", fmt.Errorf("timeout while downloading Docker image %s", image)
			}
			return "", fmt.Errorf("failed to read pull response for image %s: %w", image, readErr)
		}

		// Check if pull was successful by looking for error messages in output
		pullStr := string(pullOutput)
		if strings.Contains(pullStr, "not found") || strings.Contains(pullStr, "404") || strings.Contains(pullStr, "manifest unknown") {
			return "", fmt.Errorf("docker image %s not found in registry. Please check that the image name and tag are correct", image)
		}
		if strings.Contains(pullStr, "error") || strings.Contains(pullStr, "Error") {
			return "", fmt.Errorf("failed to pull Docker image %s: %s", image, pullStr)
		}

		fmt.Printf("Successfully pulled Docker image %s\n", image)
	} else {
		fmt.Printf("Docker image %s found locally\n", image)
	}

	// Create container config with a working directory
	config := &container.Config{
		Image:      image,
		WorkingDir: "/app",
		Tty:        true,
		OpenStdin:  true,
		StdinOnce:  false,
	}

	// Create host config
	hostConfig := &container.HostConfig{
		// Add any resource constraints here if needed
	}

	// Create the container
	resp, err := cli.ContainerCreate(
		ctx,
		config,
		hostConfig,
		nil,
		nil,
		"",
	)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	// Start the container
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	return resp.ID, nil
}
