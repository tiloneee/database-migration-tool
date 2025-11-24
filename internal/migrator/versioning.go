package migrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/thien/database-migration-tool/internal/config"
	"github.com/thien/database-migration-tool/internal/logger"
	"go.uber.org/zap"
)

// VersionManager handles versioned database migrations
type VersionManager struct {
	migrationsDir string
}

// NewVersionManager creates a new version manager
func NewVersionManager(migrationsDir string) *VersionManager {
	return &VersionManager{
		migrationsDir: migrationsDir,
	}
}

// CreateMigration generates UP migration from Ent schema
func (vm *VersionManager) CreateMigration(ctx context.Context, name string) error {
	timestamp := time.Now().Format("20060102150405")
	migrationName := fmt.Sprintf("%s_%s", timestamp, name)

	logger.Info("Generating migration from Ent schema",
		zap.String("name", migrationName))

	// Get absolute paths
	migrationsAbs, err := filepath.Abs(vm.migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to get migrations dir abs path: %w", err)
	}

	// Convert Windows path to file URL format
	migrationsDirURL := toFileURL(migrationsAbs)

	// Ensure dev database is clean before generating migration
	if err := vm.cleanDevDatabase(ctx); err != nil {
		logger.Info("⚠️  Failed to clean dev database (non-fatal)", zap.Error(err))
	}

	// Generate UP migration using local Atlas CLI (not Docker)
	// This requires Atlas CLI and Go to be installed locally
	args := []string{
		"migrate", "diff", migrationName,
		"--dir", migrationsDirURL,
		"--to", "ent://ent/schema",
		"--dev-url", "postgres://postgres:postgres@localhost:5434/atlas_dev?sslmode=disable&search_path=public",
	}

	cmd := exec.CommandContext(ctx, "atlas", args...)
	cmd.Env = os.Environ() // Ensure Go is in PATH
	output, err := cmd.CombinedOutput()

	logger.Info("Atlas output", zap.String("output", string(output)))

	if err != nil {
		return fmt.Errorf("failed to generate migration: %w\nOutput: %s", err, string(output))
	}

	// Find and rename the Atlas-generated file (it may have a different timestamp)
	// Atlas sometimes creates files like: 20251114094817_20251114164814_initial_schema.sql
	files, err := filepath.Glob(filepath.Join(vm.migrationsDir, "*_"+migrationName+".sql"))
	if err == nil && len(files) > 0 {
		// Rename to our expected format
		expectedName := filepath.Join(vm.migrationsDir, migrationName+".sql")
		if files[0] != expectedName {
			if err := os.Rename(files[0], expectedName); err != nil {
				logger.Info("⚠️  Could not rename migration file", zap.Error(err))
			}
		}
	}

	// Create empty DOWN migration file for manual editing
	// Store it in a separate directory to avoid Atlas checksum conflicts
	downDir := filepath.Join(vm.migrationsDir, "down")
	if err := os.MkdirAll(downDir, 0755); err != nil {
		return fmt.Errorf("failed to create down migrations directory: %w", err)
	}

	downFile := filepath.Join(downDir, migrationName+".down.sql")
	downContent := fmt.Sprintf(`-- Migration: %s (DOWN)
-- Generated: %s
-- TODO: Write the rollback SQL for this migration

-- Example:
-- DROP TABLE IF EXISTS new_table;
-- ALTER TABLE users DROP COLUMN IF EXISTS new_column;
`, migrationName, time.Now().Format(time.RFC3339))

	if err := os.WriteFile(downFile, []byte(downContent), 0644); err != nil {
		return fmt.Errorf("failed to create down migration: %w", err)
	}

	// Generate checksums for migration integrity (only .sql files in main dir)
	hashArgs := []string{"migrate", "hash", "--dir", migrationsDirURL}
	hashCmd := exec.CommandContext(ctx, "atlas", hashArgs...)
	hashCmd.Env = os.Environ()
	if hashOutput, err := hashCmd.CombinedOutput(); err != nil {
		logger.Info("Failed to generate checksums (non-fatal)",
			zap.Error(err),
			zap.String("output", string(hashOutput)))
		logger.Info("⚠️  Run 'atlas migrate hash' manually if needed")
	}

	logger.Info("✅ UP migration generated",
		zap.String("file", migrationName+".sql"))
	logger.Info("📝 DOWN migration template created",
		zap.String("file", "down/"+migrationName+".down.sql"))

	return nil
}

// ApplyMigrations applies pending migrations
func (vm *VersionManager) ApplyMigrations(ctx context.Context, dbConfig *config.DatabaseConfig) (int, error) {
	logger.Info("Applying migrations",
		zap.String("database", dbConfig.Database))

	migrationsAbs, err := filepath.Abs(vm.migrationsDir)
	if err != nil {
		return 0, fmt.Errorf("failed to get migrations dir abs path: %w", err)
	}

	// Use local Atlas CLI
	args := []string{
		"migrate", "apply",
		"--dir", toFileURL(migrationsAbs),
		"--url", buildDSN(dbConfig),
	}

	cmd := exec.CommandContext(ctx, "atlas", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()

	logger.Info("Atlas output", zap.String("output", string(output)))

	if err != nil {
		return 0, fmt.Errorf("migration apply failed: %w\nOutput: %s", err, string(output))
	}

	// Parse output to count applied migrations
	applied := parseAppliedCount(string(output))

	return applied, nil
}

// RollbackMigrations rolls back N migrations
func (vm *VersionManager) RollbackMigrations(ctx context.Context, dbConfig *config.DatabaseConfig, steps int) error {
	logger.Info("Rolling back migrations",
		zap.Int("steps", steps),
		zap.String("database", dbConfig.Database))

	migrationsAbs, err := filepath.Abs(vm.migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to get migrations dir abs path: %w", err)
	}

	// Use local Atlas CLI
	args := []string{
		"migrate", "down",
		fmt.Sprintf("%d", steps),
		"--dir", toFileURL(migrationsAbs),
		"--url", buildDSN(dbConfig),
		"--dev-url", "postgres://postgres:postgres@localhost:5434/atlas_dev?sslmode=disable&search_path=public",
	}

	cmd := exec.CommandContext(ctx, "atlas", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()

	logger.Info("Atlas output", zap.String("output", string(output)))

	if err != nil {
		return fmt.Errorf("rollback failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// GetStatus shows migration status
func (vm *VersionManager) GetStatus(ctx context.Context, dbConfig *config.DatabaseConfig) (string, error) {
	migrationsAbs, err := filepath.Abs(vm.migrationsDir)
	if err != nil {
		return "", fmt.Errorf("failed to get migrations dir abs path: %w", err)
	}

	// Use local Atlas CLI
	args := []string{
		"migrate", "status",
		"--dir", toFileURL(migrationsAbs),
		"--url", buildDSN(dbConfig),
	}

	cmd := exec.CommandContext(ctx, "atlas", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()

	if err != nil {
		return "", fmt.Errorf("failed to get status: %w\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

// Helper functions

// cleanDevDatabase drops and recreates the public schema in the dev database
func (vm *VersionManager) cleanDevDatabase(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "psql",
		"-h", "localhost",
		"-p", "5434",
		"-U", "postgres",
		"-d", "atlas_dev",
		"-c", "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;",
	)
	cmd.Env = append(os.Environ(), "PGPASSWORD=postgres")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If database doesn't exist, create it
		if strings.Contains(string(output), "does not exist") {
			createCmd := exec.CommandContext(ctx, "psql",
				"-h", "localhost",
				"-p", "5434",
				"-U", "postgres",
				"-d", "postgres",
				"-c", "CREATE DATABASE atlas_dev;",
			)
			createCmd.Env = append(os.Environ(), "PGPASSWORD=postgres")
			if _, err := createCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to create atlas_dev database: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to clean dev database: %w", err)
	}
	return nil
}

// toFileURL converts a file path to a file:// URL format that Atlas understands
// On Windows, converts C:\path\to\dir to file://C:/path/to/dir
func toFileURL(path string) string {
	// Convert backslashes to forward slashes
	path = strings.ReplaceAll(path, "\\", "/")
	// Ensure file:// prefix
	if !strings.HasPrefix(path, "file://") {
		return "file://" + path
	}
	return path
}

func buildDSN(cfg *config.DatabaseConfig) string {
	// Build DSN for local Atlas CLI (no need to convert to host.docker.internal)
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s&search_path=public",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.SSLMode,
	)
}

func parseAppliedCount(output string) int {
	// Look for pattern like "Migrating to version 20231114000001 (1 migrations)"
	re := regexp.MustCompile(`\((\d+) migration`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		var count int
		fmt.Sscanf(matches[1], "%d", &count)
		return count
	}

	// Alternative: count lines with "-> " which indicates applied migration
	lines := strings.Split(output, "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, "-> ") && strings.Contains(line, ".sql") {
			count++
		}
	}
	return count
}
