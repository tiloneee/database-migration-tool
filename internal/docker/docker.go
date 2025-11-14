package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/thien/database-migration-tool/internal/logger"
	"go.uber.org/zap"
)

// Client wraps Docker operations
type Client struct {
	containerName string
	composeFile   string
	autoStart     bool
}

// NewClient creates a new Docker client
func NewClient(containerName, composeFile string, autoStart bool) *Client {
	return &Client{
		containerName: containerName,
		composeFile:   composeFile,
		autoStart:     autoStart,
	}
}

// IsRunning checks if the Postgres container is running
func (c *Client) IsRunning(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "--filter", fmt.Sprintf("name=%s", c.containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check container status: %w", err)
	}

	containerName := strings.TrimSpace(string(output))
	return containerName == c.containerName, nil
}

// Start starts the Postgres container using docker-compose
func (c *Client) Start(ctx context.Context) error {
	logger.Info("Starting Postgres container", zap.String("container", c.containerName))

	// Check if docker-compose file exists
	if _, err := os.Stat(c.composeFile); os.IsNotExist(err) {
		return fmt.Errorf("docker-compose file not found: %s", c.composeFile)
	}

	// Start container
	cmd := exec.CommandContext(ctx, "docker-compose", "-f", c.composeFile, "up", "-d")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start container: %w\nOutput: %s", err, string(output))
	}

	logger.Info("Container started successfully", zap.String("output", string(output)))

	// Wait for container to be healthy
	return c.WaitForHealthy(ctx, 30*time.Second)
}

// Stop stops the Postgres container
func (c *Client) Stop(ctx context.Context) error {
	logger.Info("Stopping Postgres container", zap.String("container", c.containerName))

	cmd := exec.CommandContext(ctx, "docker-compose", "-f", c.composeFile, "stop")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop container: %w\nOutput: %s", err, string(output))
	}

	logger.Info("Container stopped successfully")
	return nil
}

// Recreate recreates the Postgres container (useful for clean slate)
func (c *Client) Recreate(ctx context.Context) error {
	logger.Info("Recreating Postgres container", zap.String("container", c.containerName))

	// Stop and remove existing container
	cmd := exec.CommandContext(ctx, "docker-compose", "-f", c.composeFile, "down", "-v")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Warn("Failed to stop existing container", zap.Error(err), zap.String("output", string(output)))
	}

	// Start fresh container
	return c.Start(ctx)
}

// WaitForHealthy waits for the container to be healthy and accepting connections
func (c *Client) WaitForHealthy(ctx context.Context, timeout time.Duration) error {
	logger.Info("Waiting for Postgres to be ready", zap.Duration("timeout", timeout))

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for container to be healthy")
			}

			// Check if container is running
			running, err := c.IsRunning(ctx)
			if err != nil {
				logger.Debug("Failed to check container status", zap.Error(err))
				continue
			}

			if !running {
				logger.Debug("Container not yet running")
				continue
			}

			// Check if PostgreSQL is accepting connections
			cmd := exec.CommandContext(ctx, "docker", "exec", c.containerName, "pg_isready", "-U", "postgres")
			if err := cmd.Run(); err == nil {
				logger.Info("Postgres is ready")
				return nil
			}

			logger.Debug("Postgres not ready yet, retrying...")
		}
	}
}

// EnsureRunning ensures the container is running, starting it if necessary
func (c *Client) EnsureRunning(ctx context.Context) error {
	running, err := c.IsRunning(ctx)
	if err != nil {
		return err
	}

	if running {
		logger.Info("Postgres container is already running")
		return nil
	}

	if !c.autoStart {
		return fmt.Errorf("postgres container is not running and auto-start is disabled")
	}

	logger.Info("Postgres container is not running, starting it now")
	return c.Start(ctx)
}

// GetLogs retrieves container logs
func (c *Client) GetLogs(ctx context.Context, tail int) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", fmt.Sprintf("%d", tail), c.containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	return string(output), nil
}
