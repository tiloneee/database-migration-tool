package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/thien/database-migration-tool/internal/anonymizer"
	"github.com/thien/database-migration-tool/internal/config"
	"github.com/thien/database-migration-tool/internal/logger"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// DataMigrator handles data migration between databases
type DataMigrator struct {
	remoteDB   *sql.DB
	localDB    *sql.DB
	config     *config.MigrationConfig
	anonymizer *anonymizer.Anonymizer
}

// NewDataMigrator creates a new data migrator
func NewDataMigrator(remoteDB, localDB *sql.DB, cfg *config.MigrationConfig) *DataMigrator {
	return &DataMigrator{
		remoteDB:   remoteDB,
		localDB:    localDB,
		config:     cfg,
		anonymizer: anonymizer.NewAnonymizer(),
	}
}

// MigrateResult holds migration results
type MigrateResult struct {
	Table        string
	RowsMigrated int64
	Success      bool
	Error        error
}

// MigrateAll migrates all tables or specified tables
func (m *DataMigrator) MigrateAll(ctx context.Context) ([]MigrateResult, error) {
	tables, err := m.getTablesToMigrate(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}

	logger.Info("Starting data migration", zap.Int("table_count", len(tables)))

	var results []MigrateResult
	for _, table := range tables {
		result := m.migrateTable(ctx, table)
		results = append(results, result)

		if !result.Success {
			logger.Error("Failed to migrate table", 
				zap.String("table", table),
				zap.Error(result.Error))
		} else {
			logger.Info("Successfully migrated table",
				zap.String("table", table),
				zap.Int64("rows", result.RowsMigrated))
		}
	}

	return results, nil
}

// getTablesToMigrate returns list of tables to migrate
func (m *DataMigrator) getTablesToMigrate(ctx context.Context) ([]string, error) {
	// If specific tables are configured, use those
	if len(m.config.Tables) > 0 {
		return m.config.Tables, nil
	}
	// Otherwise, get all tables from remote DB
	query := `
		SELECT tablename 
		FROM pg_tables 
		WHERE schemaname = 'public'
		ORDER BY tablename
	`

	rows, err := m.remoteDB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	excludeMap := make(map[string]bool)
	for _, table := range m.config.ExcludeTables {
		excludeMap[table] = true
	}

	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}

		// Skip excluded tables
		if !excludeMap[table] {
			tables = append(tables, table)
		}
	}

	return tables, rows.Err()
}

// migrateTable migrates a single table
func (m *DataMigrator) migrateTable(ctx context.Context, table string) MigrateResult {
	result := MigrateResult{
		Table:   table,
		Success: false,
	}

	// Truncate destination table if configured
	if m.config.TruncateTables {
		if err := m.truncateTable(ctx, table); err != nil {
			result.Error = fmt.Errorf("failed to truncate table: %w", err)
			return result
		}
	}

	// Get column names
	columns, err := m.getTableColumns(ctx, table)
	if err != nil {
		result.Error = fmt.Errorf("failed to get columns: %w", err)
		return result
	}

	// Read data from remote
	selectQuery := fmt.Sprintf("SELECT %s FROM %s", strings.Join(columns, ", "), table)
	rows, err := m.remoteDB.QueryContext(ctx, selectQuery)
	if err != nil {
		result.Error = fmt.Errorf("failed to query remote table: %w", err)
		return result
	}
	defer rows.Close()

	// Prepare insert statement
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	insertQuery := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	stmt, err := m.localDB.PrepareContext(ctx, insertQuery)
	if err != nil {
		result.Error = fmt.Errorf("failed to prepare insert statement: %w", err)
		return result
	}
	defer stmt.Close()

	// Begin transaction
	tx, err := m.localDB.BeginTx(ctx, nil)
	if err != nil {
		result.Error = fmt.Errorf("failed to begin transaction: %w", err)
		return result
	}

	var rowCount int64
	batchCount := 0

	for rows.Next() {
		// Scan row
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			tx.Rollback()
			result.Error = fmt.Errorf("failed to scan row: %w", err)
			return result
		}

		// Anonymize if configured
		if m.config.Anonymize {
			for i, col := range columns {
				values[i] = m.anonymizer.AnonymizeValue(col, values[i])
			}
		}

		// Insert into local DB
		if _, err := tx.StmtContext(ctx, stmt).ExecContext(ctx, values...); err != nil {
			tx.Rollback()
			result.Error = fmt.Errorf("failed to insert row: %w", err)
			return result
		}

		rowCount++
		batchCount++

		// Commit in batches
		if batchCount >= m.config.BatchSize {
			if err := tx.Commit(); err != nil {
				result.Error = fmt.Errorf("failed to commit batch: %w", err)
				return result
			}

			// Start new transaction
			tx, err = m.localDB.BeginTx(ctx, nil)
			if err != nil {
				result.Error = fmt.Errorf("failed to begin new transaction: %w", err)
				return result
			}

			batchCount = 0
			logger.Debug("Committed batch", zap.String("table", table), zap.Int64("rows", rowCount))
		}
	}

	// Commit remaining rows
	if err := tx.Commit(); err != nil {
		result.Error = fmt.Errorf("failed to commit final batch: %w", err)
		return result
	}

	if err := rows.Err(); err != nil {
		result.Error = fmt.Errorf("error during row iteration: %w", err)
		return result
	}

	result.RowsMigrated = rowCount
	result.Success = true
	return result
}

// truncateTable truncates a table in the local database
func (m *DataMigrator) truncateTable(ctx context.Context, table string) error {
	query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
	_, err := m.localDB.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to truncate table %s: %w", table, err)
	}
	logger.Debug("Truncated table", zap.String("table", table))
	return nil
}

// getTableColumns returns column names for a table
func (m *DataMigrator) getTableColumns(ctx context.Context, table string) ([]string, error) {
	query := `
		SELECT column_name 
		FROM information_schema.columns 
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position
	`

	rows, err := m.remoteDB.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %w", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, fmt.Errorf("failed to scan column: %w", err)
		}
		columns = append(columns, column)
	}

	return columns, rows.Err()
}
