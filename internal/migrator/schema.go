package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"os/exec"

	"github.com/thien/database-migration-tool/internal/config"
	"github.com/thien/database-migration-tool/internal/logger"
	"go.uber.org/zap"
)

// SchemaMigrator handles schema migration using Atlas
type SchemaMigrator struct {
	remoteDB  *sql.DB
	localDB   *sql.DB
	remoteCfg *config.DatabaseConfig
	localCfg  *config.DatabaseConfig
}

// NewSchemaMigrator creates a new schema migrator
func NewSchemaMigrator(remoteDB, localDB *sql.DB, remoteCfg, localCfg *config.DatabaseConfig) *SchemaMigrator {
	return &SchemaMigrator{
		remoteDB:  remoteDB,
		localDB:   localDB,
		remoteCfg: remoteCfg,
		localCfg:  localCfg,
	}
}

// Migrate performs schema migration from remote to local
func (s *SchemaMigrator) Migrate(ctx context.Context, dryRun bool) error {
	logger.Info("Starting schema migration", zap.Bool("dry_run", dryRun))

	// Use Atlas CLI to diff and apply schema
	if err := s.applyWithAtlas(ctx, dryRun); err != nil {
		return fmt.Errorf("atlas migration failed: %w", err)
	}

	logger.Info("Schema migration completed successfully")
	return nil
}

// applyWithAtlas uses Atlas CLI via Docker to diff and apply schema
func (s *SchemaMigrator) applyWithAtlas(ctx context.Context, dryRun bool) error {
	// First, try to use Atlas via Docker
	if err := s.applyWithAtlasDocker(ctx, dryRun); err != nil {
		logger.Warn("Atlas Docker failed, falling back to pg_dump", zap.Error(err))
		return s.applyWithPgDump(ctx)
	}
	return nil
}

// applyWithAtlasDocker uses Atlas CLI from Docker container
func (s *SchemaMigrator) applyWithAtlasDocker(ctx context.Context, dryRun bool) error {
	localURL := s.convertDSNForDocker(s.localCfg)
	remoteURL := s.convertDSNForDocker(s.remoteCfg)

	args := []string{
		"run", "--rm",
		"--network", "host",
		"arigaio/atlas:latest",
		"schema", "apply",
		"--url", localURL,
		"--to", remoteURL,
	}

	if dryRun {
		args = append(args, "--dry-run")
	} else {
		args = append(args, "--auto-approve")
	}

	logger.Debug("Running Atlas via Docker", zap.Strings("args", args))

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()

	logger.Info("Atlas output", zap.String("output", string(output)))

	if err != nil {
		return fmt.Errorf("atlas docker command failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// applyWithPgDump uses pg_dump and psql as fallback
func (s *SchemaMigrator) applyWithPgDump(ctx context.Context) error {
	logger.Info("Using pg_dump/psql for schema migration")

	// Use Docker to run pg_dump from remote and psql to local
	dumpCmd := fmt.Sprintf(
		"docker exec db_remote pg_dump -U %s -d %s --schema-only",
		s.remoteCfg.User,
		s.remoteCfg.Database,
	)

	restoreCmd := fmt.Sprintf(
		"docker exec -i db_local psql -U %s -d %s",
		s.localCfg.User,
		s.localCfg.Database,
	)

	// Combine commands with pipe
	fullCmd := fmt.Sprintf("%s | %s", dumpCmd, restoreCmd)

	logger.Debug("Running pg_dump pipeline", zap.String("command", fullCmd))

	cmd := exec.CommandContext(ctx, "bash", "-c", fullCmd)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("pg_dump/psql pipeline failed: %w\nOutput: %s", err, string(output))
	}

	logger.Info("Schema migrated successfully using pg_dump")
	return nil
}

// convertDSNForDocker converts DSN to be accessible from Docker container
func (s *SchemaMigrator) convertDSNForDocker(cfg *config.DatabaseConfig) string {
	host := cfg.Host
	// If localhost, use host.docker.internal for Docker
	if host == "localhost" || host == "127.0.0.1" {
		host = "host.docker.internal"
	}

	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		host,
		cfg.Port,
		cfg.Database,
		cfg.SSLMode,
	)
}

// Diff generates a schema diff without applying
func (s *SchemaMigrator) Diff(ctx context.Context) (string, error) {
	logger.Info("Generating schema diff")

	// Use Atlas via Docker
	localURL := s.convertDSNForDocker(s.localCfg)
	remoteURL := s.convertDSNForDocker(s.remoteCfg)

	args := []string{
		"run", "--rm",
		"--network", "host",
		"arigaio/atlas:latest",
		"schema", "diff",
		"--from", localURL,
		"--to", remoteURL,
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return "", fmt.Errorf("failed to generate diff: %w\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

// Inspect inspects the schema of a database
func (s *SchemaMigrator) Inspect(ctx context.Context, remote bool) (string, error) {
	var cfg *config.DatabaseConfig
	var dbType string

	if remote {
		cfg = s.remoteCfg
		dbType = "remote"
	} else {
		cfg = s.localCfg
		dbType = "local"
	}

	logger.Info("Inspecting schema", zap.String("database", dbType))

	// Use Atlas via Docker
	dsn := s.convertDSNForDocker(cfg)

	args := []string{
		"run", "--rm",
		"--network", "host",
		"arigaio/atlas:latest",
		"schema", "inspect",
		"--url", dsn,
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return "", fmt.Errorf("failed to inspect schema: %w\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

// ExportSchema exports the remote schema to an SQL file
func (s *SchemaMigrator) ExportSchema(ctx context.Context, outputFile string) error {
	logger.Info("Exporting schema to file", zap.String("file", outputFile))

	// Use pg_dump to export schema only
	args := []string{
		"-h", s.remoteCfg.Host,
		"-p", fmt.Sprintf("%d", s.remoteCfg.Port),
		"-U", s.remoteCfg.User,
		"-d", s.remoteCfg.Database,
		"--schema-only",
		"-f", outputFile,
	}

	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("PGPASSWORD=%s", s.remoteCfg.Password))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w\nOutput: %s", err, string(output))
	}

	logger.Info("Schema exported successfully", zap.String("file", outputFile))
	return nil
}

// ImportSchema imports schema from an SQL file to local database
func (s *SchemaMigrator) ImportSchema(ctx context.Context, inputFile string) error {
	logger.Info("Importing schema from file", zap.String("file", inputFile))

	args := []string{
		"-h", s.localCfg.Host,
		"-p", fmt.Sprintf("%d", s.localCfg.Port),
		"-U", s.localCfg.User,
		"-d", s.localCfg.Database,
		"-f", inputFile,
	}

	cmd := exec.CommandContext(ctx, "psql", args...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("PGPASSWORD=%s", s.localCfg.Password))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("psql import failed: %w\nOutput: %s", err, string(output))
	}

	logger.Info("Schema imported successfully")
	return nil
}
