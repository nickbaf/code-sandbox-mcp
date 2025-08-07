package tools

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/docker/docker/client"
)

// createDockerClient creates a Docker client with fallback options for different environments
func createDockerClient() (*client.Client, error) {
	// First try the standard FromEnv approach
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err == nil {
		// Test if the client can actually connect
		_, pingErr := cli.Ping(context.Background())
		if pingErr == nil {
			return cli, nil
		}
		cli.Close()
	}

	// If FromEnv failed, try common Docker socket paths
	socketPaths := []string{
		"/var/run/docker.sock",
	}

	// Add user-specific paths for common Docker alternatives
	if currentUser, userErr := user.Current(); userErr == nil {
		userPaths := []string{
			filepath.Join(currentUser.HomeDir, ".rd", "docker.sock"),                // Rancher Desktop
			filepath.Join(currentUser.HomeDir, ".docker", "run", "docker.sock"),     // Docker Desktop
			filepath.Join(currentUser.HomeDir, ".colima", "default", "docker.sock"), // Colima
		}
		socketPaths = append(socketPaths, userPaths...)
	}

	// Try each socket path
	for _, socketPath := range socketPaths {
		if _, statErr := os.Stat(socketPath); statErr == nil {
			// Socket exists, try to connect
			cli, err := client.NewClientWithOpts(
				client.WithHost("unix://"+socketPath),
				client.WithAPIVersionNegotiation(),
			)
			if err == nil {
				// Test if the client can actually connect
				_, pingErr := cli.Ping(context.Background())
				if pingErr == nil {
					return cli, nil
				}
				cli.Close()
			}
		}
	}

	return nil, fmt.Errorf("could not connect to Docker daemon. Tried standard connection and socket paths: %v", socketPaths)
}
