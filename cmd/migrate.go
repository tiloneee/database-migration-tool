package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"github.com/thien/database-migration-tool/internal/config"
	"github.com/thien/database-migration-tool/internal/docker"
	"github.com/thien/database-migration-tool/internal/logger"
	"github.com/thien/database-migration-tool/internal/migrator"
	"github.com/thien/database-migration-tool/internal/verifier"
	"go.uber.org/zap"
)

var (
	cfgFile      string
	cfg          *config.Config
	dockerClient *docker.Client
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "migrate",
	Short: "PostgreSQL database migration tool",
	Long: `A CLI tool to migrate schema and data from a remote PostgreSQL database 
to a local PostgreSQL running in Docker.

Features:
- Schema migration using AtlasGo
- Data migration with optional anonymization
- Docker container management
- Data verification and reporting`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Load configuration
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		// Initialize logger
		if err := logger.Init(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.OutputPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
			os.Exit(1)
		}

		// Initialize Docker client
		dockerClient = docker.NewClient(
			cfg.Docker.ContainerName,
			cfg.Docker.ComposeFile,
			cfg.Docker.AutoStart,
		)
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		logger.Close()
	},
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./config.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
}

// pullCmd migrates both schema and data
var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull schema and data from remote database",
	Long:  "Performs complete migration: schema sync and data transfer from remote to local database",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := setupContext()

		logger.Info("Starting complete database migration")

		// Ensure Docker container is running
		if err := dockerClient.EnsureRunning(ctx); err != nil {
			logger.Fatal("Failed to ensure Docker container is running", zap.Error(err))
		}

		// Wait a bit for database to be fully ready
		time.Sleep(2 * time.Second)

		// Connect to databases
		remoteDB, localDB := connectDatabases(ctx)
		defer remoteDB.Close()
		defer localDB.Close()

		// Migrate schema
		logger.Info("Step 1/3: Migrating schema")
		schemaMigrator := migrator.NewSchemaMigrator(remoteDB, localDB, &cfg.Remote, &cfg.Local)

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if err := schemaMigrator.Migrate(ctx, dryRun); err != nil {
			logger.Fatal("Schema migration failed", zap.Error(err))
		}

		if dryRun {
			logger.Info("Dry run completed - no changes applied")
			return
		}

		// Migrate data
		logger.Info("Step 2/3: Migrating data")
		dataMigrator := migrator.NewDataMigrator(remoteDB, localDB, &cfg.Migration)
		results, err := dataMigrator.MigrateAll(ctx)
		if err != nil {
			logger.Fatal("Data migration failed", zap.Error(err))
		}

		// Verify migration
		logger.Info("Step 3/3: Verifying migration")
		v := verifier.NewVerifier(remoteDB, localDB)

		var tables []string
		for _, r := range results {
			if r.Success {
				tables = append(tables, r.Table)
			}
		}

		verifyResults, err := v.VerifyAll(ctx, tables)
		if err != nil {
			logger.Warn("Verification encountered errors", zap.Error(err))
		}

		// Generate and display report
		report := v.GenerateReport(verifyResults)
		fmt.Println(report)

		logger.Info("Migration completed successfully!")
	},
}

// schemaCmd handles schema-only migration
var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Migrate schema only",
	Long:  "Sync database schema from remote to local using AtlasGo",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := setupContext()

		// Ensure Docker container is running
		if err := dockerClient.EnsureRunning(ctx); err != nil {
			logger.Fatal("Failed to ensure Docker container is running", zap.Error(err))
		}

		time.Sleep(2 * time.Second)

		remoteDB, localDB := connectDatabases(ctx)
		defer remoteDB.Close()
		defer localDB.Close()

		schemaMigrator := migrator.NewSchemaMigrator(remoteDB, localDB, &cfg.Remote, &cfg.Local)

		action, _ := cmd.Flags().GetString("action")

		switch action {
		case "diff":
			diff, err := schemaMigrator.Diff(ctx)
			if err != nil {
				logger.Fatal("Failed to generate diff", zap.Error(err))
			}
			fmt.Println(diff)
		case "apply":
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			if err := schemaMigrator.Migrate(ctx, dryRun); err != nil {
				logger.Fatal("Schema migration failed", zap.Error(err))
			}
			logger.Info("Schema migration completed")
		case "export":
			outputFile, _ := cmd.Flags().GetString("output")
			if err := schemaMigrator.ExportSchema(ctx, outputFile); err != nil {
				logger.Fatal("Failed to export schema", zap.Error(err))
			}
			logger.Info("Schema exported successfully", zap.String("file", outputFile))
		default:
			logger.Fatal("Invalid action. Use: diff, apply, or export")
		}
	},
}

// dataCmd handles data-only migration
var dataCmd = &cobra.Command{
	Use:   "data",
	Short: "Migrate data only",
	Long:  "Transfer data from remote to local database",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := setupContext()

		if err := dockerClient.EnsureRunning(ctx); err != nil {
			logger.Fatal("Failed to ensure Docker container is running", zap.Error(err))
		}

		time.Sleep(2 * time.Second)

		remoteDB, localDB := connectDatabases(ctx)
		defer remoteDB.Close()
		defer localDB.Close()

		dataMigrator := migrator.NewDataMigrator(remoteDB, localDB, &cfg.Migration)
		results, err := dataMigrator.MigrateAll(ctx)
		if err != nil {
			logger.Fatal("Data migration failed", zap.Error(err))
		}

		// Summary
		successful := 0
		totalRows := int64(0)
		for _, r := range results {
			if r.Success {
				successful++
				totalRows += r.RowsMigrated
			}
		}

		logger.Info("Data migration completed",
			zap.Int("successful_tables", successful),
			zap.Int("total_tables", len(results)),
			zap.Int64("total_rows", totalRows))
	},
}

// verifyCmd verifies migration integrity
var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify migration integrity",
	Long:  "Compare remote and local databases to verify data consistency",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := setupContext()

		remoteDB, localDB := connectDatabases(ctx)
		defer remoteDB.Close()
		defer localDB.Close()

		v := verifier.NewVerifier(remoteDB, localDB)

		// Verify schema
		if err := v.VerifySchema(ctx); err != nil {
			logger.Error("Schema verification failed", zap.Error(err))
		}

		// Get tables to verify
		tables := cfg.Migration.Tables
		if len(tables) == 0 {
			// Get all tables if none specified
			query := "SELECT tablename FROM pg_tables WHERE schemaname = 'public'"
			rows, err := remoteDB.QueryContext(ctx, query)
			if err != nil {
				logger.Fatal("Failed to get tables", zap.Error(err))
			}
			defer rows.Close()

			for rows.Next() {
				var table string
				if err := rows.Scan(&table); err != nil {
					logger.Fatal("Failed to scan table name", zap.Error(err))
				}
				tables = append(tables, table)
			}
		}

		// Verify data
		results, err := v.VerifyAll(ctx, tables)
		if err != nil {
			logger.Warn("Verification encountered errors", zap.Error(err))
		}

		// Display report
		report := v.GenerateReport(results)
		fmt.Println(report)
	},
}

// dockerCmd manages Docker container
var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Manage Docker container",
	Long:  "Start, stop, or recreate the local PostgreSQL Docker container",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := setupContext()

		action, _ := cmd.Flags().GetString("action")

		switch action {
		case "start":
			if err := dockerClient.Start(ctx); err != nil {
				logger.Fatal("Failed to start container", zap.Error(err))
			}
			logger.Info("Container started successfully")
		case "stop":
			if err := dockerClient.Stop(ctx); err != nil {
				logger.Fatal("Failed to stop container", zap.Error(err))
			}
			logger.Info("Container stopped successfully")
		case "restart":
			if err := dockerClient.Stop(ctx); err != nil {
				logger.Warn("Failed to stop container", zap.Error(err))
			}
			if err := dockerClient.Start(ctx); err != nil {
				logger.Fatal("Failed to start container", zap.Error(err))
			}
			logger.Info("Container restarted successfully")
		case "recreate":
			if err := dockerClient.Recreate(ctx); err != nil {
				logger.Fatal("Failed to recreate container", zap.Error(err))
			}
			logger.Info("Container recreated successfully")
		case "status":
			running, err := dockerClient.IsRunning(ctx)
			if err != nil {
				logger.Fatal("Failed to check container status", zap.Error(err))
			}
			if running {
				fmt.Println("Container is running")
			} else {
				fmt.Println("Container is not running")
			}
		case "logs":
			logs, err := dockerClient.GetLogs(ctx, 100)
			if err != nil {
				logger.Fatal("Failed to get logs", zap.Error(err))
			}
			fmt.Println(logs)
		default:
			logger.Fatal("Invalid action. Use: start, stop, restart, recreate, status, or logs")
		}
	},
}

// NEW: migrate command for schema versioning
var migrateSchemaCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage database schema migrations",
	Long:  "Create, apply, and rollback versioned database migrations using Ent + Atlas",
}

// migrate create - Generate new migration
var migrateCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create new migration from Ent schema changes",
	Long:  "Generates UP migration automatically. You must write DOWN migration manually.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := setupContext()
		migrationName := args[0]

		logger.Info("Creating new migration", zap.String("name", migrationName))

		versionMgr := migrator.NewVersionManager("./migrations")

		if err := versionMgr.CreateMigration(ctx, migrationName); err != nil {
			logger.Fatal("Failed to create migration", zap.Error(err))
		}

		logger.Info("✅ Migration created successfully!")
		logger.Info("⚠️  IMPORTANT: Write the DOWN migration manually!")
		fmt.Printf("\n📝 Edit the DOWN migration: migrations/*_%s.down.sql\n", migrationName)
	},
}

// migrate up - Apply migrations
var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply pending migrations",
	Long:  "Applies all pending migrations to bring database schema up to date",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := setupContext()

		target, _ := cmd.Flags().GetString("target")

		logger.Info("Applying migrations", zap.String("target", target))

		versionMgr := migrator.NewVersionManager("./migrations")

		var targetDB *config.DatabaseConfig
		if target == "local" {
			targetDB = &cfg.Local
		} else {
			targetDB = &cfg.Remote
		}

		applied, err := versionMgr.ApplyMigrations(ctx, targetDB)
		if err != nil {
			logger.Fatal("Migration failed", zap.Error(err))
		}

		logger.Info("✅ Migrations applied successfully!", zap.Int("count", applied))
	},
}

// migrate down - Rollback migrations
var migrateDownCmd = &cobra.Command{
	Use:   "down [steps]",
	Short: "Rollback migrations",
	Long:  "Rolls back the specified number of migrations (default: 1)",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := setupContext()

		steps := 1
		if len(args) > 0 {
			fmt.Sscanf(args[0], "%d", &steps)
		}

		target, _ := cmd.Flags().GetString("target")

		logger.Info("Rolling back migrations",
			zap.Int("steps", steps),
			zap.String("target", target))

		versionMgr := migrator.NewVersionManager("./migrations")

		var targetDB *config.DatabaseConfig
		if target == "local" {
			targetDB = &cfg.Local
		} else {
			targetDB = &cfg.Remote
		}

		if err := versionMgr.RollbackMigrations(ctx, targetDB, steps); err != nil {
			logger.Fatal("Rollback failed", zap.Error(err))
		}

		logger.Info("✅ Rollback completed successfully!")
	},
}

// migrate status - Show migration status
var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	Long:  "Displays applied and pending migrations",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := setupContext()

		target, _ := cmd.Flags().GetString("target")

		versionMgr := migrator.NewVersionManager("./migrations")

		var targetDB *config.DatabaseConfig
		if target == "local" {
			targetDB = &cfg.Local
		} else {
			targetDB = &cfg.Remote
		}

		status, err := versionMgr.GetStatus(ctx, targetDB)
		if err != nil {
			logger.Fatal("Failed to get status", zap.Error(err))
		}

		fmt.Println(status)
	},
}

// NEW: push command - Like git push (local -> remote)
var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push changes to remote database",
	Long:  "Pushes schema migrations and data from local to remote database (like git push)",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := setupContext()

		schemaOnly, _ := cmd.Flags().GetBool("schema-only")
		dataOnly, _ := cmd.Flags().GetBool("data-only")

		logger.Info("🚀 Pushing to remote database...")

		// Apply schema migrations to remote
		if !dataOnly {
			logger.Info("Step 1/2: Pushing schema migrations to remote")
			versionMgr := migrator.NewVersionManager("./migrations")
			applied, err := versionMgr.ApplyMigrations(ctx, &cfg.Remote)
			if err != nil {
				logger.Fatal("Failed to push schema", zap.Error(err))
			}
			logger.Info("✅ Schema pushed", zap.Int("migrations", applied))
		}

		// Sync data to remote
		if !schemaOnly {
			logger.Info("Step 2/2: Pushing data to remote")

			localDB, remoteDB := connectDatabases(ctx)
			defer localDB.Close()
			defer remoteDB.Close()

			dataMigrator := migrator.NewDataMigrator(localDB, remoteDB, &cfg.Migration)
			results, err := dataMigrator.MigrateAll(ctx)
			if err != nil {
				logger.Fatal("Failed to push data", zap.Error(err))
			}

			successful := 0
			totalRows := int64(0)
			for _, r := range results {
				if r.Success {
					successful++
					totalRows += r.RowsMigrated
				}
			}

			logger.Info("✅ Data pushed",
				zap.Int("tables", successful),
				zap.Int64("rows", totalRows))
		}

		logger.Info("🎉 Push completed successfully!")
	},
}

// pullCmd updated - Now pulls from remote to local (like git pull)
var newPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull changes from remote database",
	Long:  "Pulls schema migrations and data from remote to local database (like git pull)",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := setupContext()

		schemaOnly, _ := cmd.Flags().GetBool("schema-only")
		dataOnly, _ := cmd.Flags().GetBool("data-only")

		logger.Info("⬇️  Pulling from remote database...")

		// Ensure Docker container is running
		if err := dockerClient.EnsureRunning(ctx); err != nil {
			logger.Fatal("Failed to ensure Docker container is running", zap.Error(err))
		}

		time.Sleep(2 * time.Second)

		// Apply schema migrations to local
		if !dataOnly {
			logger.Info("Step 1/2: Pulling schema migrations to local")
			versionMgr := migrator.NewVersionManager("./migrations")
			applied, err := versionMgr.ApplyMigrations(ctx, &cfg.Local)
			if err != nil {
				logger.Fatal("Failed to pull schema", zap.Error(err))
			}
			logger.Info("✅ Schema pulled", zap.Int("migrations", applied))
		}

		// Sync data to local
		if !schemaOnly {
			logger.Info("Step 2/2: Pulling data to local")

			remoteDB, localDB := connectDatabases(ctx)
			defer remoteDB.Close()
			defer localDB.Close()

			dataMigrator := migrator.NewDataMigrator(remoteDB, localDB, &cfg.Migration)
			results, err := dataMigrator.MigrateAll(ctx)
			if err != nil {
				logger.Fatal("Failed to pull data", zap.Error(err))
			}

			successful := 0
			totalRows := int64(0)
			for _, r := range results {
				if r.Success {
					successful++
					totalRows += r.RowsMigrated
				}
			}

			logger.Info("✅ Data pulled",
				zap.Int("tables", successful),
				zap.Int64("rows", totalRows))
		}

		logger.Info("🎉 Pull completed successfully!")
	},
}

func init() {
	// Migration command tree
	migrateSchemaCmd.AddCommand(migrateCreateCmd)
	migrateSchemaCmd.AddCommand(migrateUpCmd)
	migrateSchemaCmd.AddCommand(migrateDownCmd)
	migrateSchemaCmd.AddCommand(migrateStatusCmd)

	// Flags for migrate up/down/status
	migrateUpCmd.Flags().String("target", "local", "Target database: local or remote")
	migrateDownCmd.Flags().String("target", "local", "Target database: local or remote")
	migrateStatusCmd.Flags().String("target", "local", "Target database: local or remote")

	rootCmd.AddCommand(migrateSchemaCmd)

	// Push command (local -> remote)
	pushCmd.Flags().Bool("schema-only", false, "Push schema migrations only")
	pushCmd.Flags().Bool("data-only", false, "Push data only")
	rootCmd.AddCommand(pushCmd)

	// Pull command (remote -> local) - Replace old pullCmd
	newPullCmd.Flags().Bool("schema-only", false, "Pull schema migrations only")
	newPullCmd.Flags().Bool("data-only", false, "Pull data only")
	rootCmd.AddCommand(newPullCmd)

	// Schema command flags (keep for backward compatibility)
	schemaCmd.Flags().String("action", "apply", "Action to perform: diff, apply, or export")
	schemaCmd.Flags().Bool("dry-run", false, "Show what would be done without applying changes")
	schemaCmd.Flags().String("output", "schema.sql", "Output file for export action")
	rootCmd.AddCommand(schemaCmd)

	// Data command
	rootCmd.AddCommand(dataCmd)

	// Verify command
	rootCmd.AddCommand(verifyCmd)

	// Docker command flags
	dockerCmd.Flags().String("action", "status", "Action to perform: start, stop, restart, recreate, status, or logs")
	rootCmd.AddCommand(dockerCmd)
}

// Helper functions

func setupContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Received interrupt signal, shutting down...")
		cancel()
	}()

	return ctx
}

func connectDatabases(ctx context.Context) (*sql.DB, *sql.DB) {
	logger.Info("Connecting to remote database", zap.String("host", cfg.Remote.Host))
	remoteDB, err := sql.Open("postgres", cfg.Remote.ConnectionString())
	if err != nil {
		logger.Fatal("Failed to connect to remote database", zap.Error(err))
	}

	if err := remoteDB.PingContext(ctx); err != nil {
		logger.Fatal("Failed to ping remote database", zap.Error(err))
	}

	logger.Info("Connecting to local database", zap.String("host", cfg.Local.Host))
	localDB, err := sql.Open("postgres", cfg.Local.ConnectionString())
	if err != nil {
		logger.Fatal("Failed to connect to local database", zap.Error(err))
	}

	if err := localDB.PingContext(ctx); err != nil {
		logger.Fatal("Failed to ping local database", zap.Error(err))
	}

	logger.Info("Database connections established")
	return remoteDB, localDB
}
