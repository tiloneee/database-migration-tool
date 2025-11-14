package verifier

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/thien/database-migration-tool/internal/logger"
	"go.uber.org/zap"
)

// Verifier handles data integrity verification
type Verifier struct {
	remoteDB *sql.DB
	localDB  *sql.DB
}

// NewVerifier creates a new verifier
func NewVerifier(remoteDB, localDB *sql.DB) *Verifier {
	return &Verifier{
		remoteDB: remoteDB,
		localDB:  localDB,
	}
}

// VerificationResult holds verification results for a table
type VerificationResult struct {
	Table      string
	RemoteRows int64
	LocalRows  int64
	Match      bool
	RowDiff    int64
	Error      error
}

// VerifyAll verifies all tables
func (v *Verifier) VerifyAll(ctx context.Context, tables []string) ([]VerificationResult, error) {
	logger.Info("Starting verification", zap.Int("table_count", len(tables)))

	var results []VerificationResult

	for _, table := range tables {
		result := v.verifyTable(ctx, table)
		results = append(results, result)

		if result.Error != nil {
			logger.Error("Verification error",
				zap.String("table", table),
				zap.Error(result.Error))
		} else if !result.Match {
			logger.Warn("Row count mismatch",
				zap.String("table", table),
				zap.Int64("remote", result.RemoteRows),
				zap.Int64("local", result.LocalRows),
				zap.Int64("diff", result.RowDiff))
		} else {
			logger.Info("Verification passed",
				zap.String("table", table),
				zap.Int64("rows", result.LocalRows))
		}
	}

	return results, nil
}

// verifyTable verifies a single table
func (v *Verifier) verifyTable(ctx context.Context, table string) VerificationResult {
	result := VerificationResult{
		Table: table,
	}

	// Get remote row count
	remoteCount, err := v.getRowCount(ctx, v.remoteDB, table)
	if err != nil {
		result.Error = fmt.Errorf("failed to get remote row count: %w", err)
		return result
	}
	result.RemoteRows = remoteCount

	// Get local row count
	localCount, err := v.getRowCount(ctx, v.localDB, table)
	if err != nil {
		result.Error = fmt.Errorf("failed to get local row count: %w", err)
		return result
	}
	result.LocalRows = localCount

	// Compare
	result.RowDiff = result.RemoteRows - result.LocalRows
	result.Match = (result.RowDiff == 0)

	return result
}

// getRowCount gets the row count for a table
func (v *Verifier) getRowCount(ctx context.Context, db *sql.DB, table string) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	var count int64
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// VerifySchema verifies that schema exists in both databases
func (v *Verifier) VerifySchema(ctx context.Context) error {
	logger.Info("Verifying schema consistency")

	// Get tables from remote
	remoteTables, err := v.getTables(ctx, v.remoteDB)
	if err != nil {
		return fmt.Errorf("failed to get remote tables: %w", err)
	}

	// Get tables from local
	localTables, err := v.getTables(ctx, v.localDB)
	if err != nil {
		return fmt.Errorf("failed to get local tables: %w", err)
	}

	// Convert to maps for comparison
	remoteMap := make(map[string]bool)
	for _, t := range remoteTables {
		remoteMap[t] = true
	}

	localMap := make(map[string]bool)
	for _, t := range localTables {
		localMap[t] = true
	}

	// Find missing tables
	var missingInLocal []string
	for table := range remoteMap {
		if !localMap[table] {
			missingInLocal = append(missingInLocal, table)
		}
	}

	if len(missingInLocal) > 0 {
		logger.Warn("Tables missing in local database", zap.Strings("tables", missingInLocal))
		return fmt.Errorf("schema mismatch: %d tables missing in local database", len(missingInLocal))
	}

	logger.Info("Schema verification passed",
		zap.Int("remote_tables", len(remoteTables)),
		zap.Int("local_tables", len(localTables)))

	return nil
}

// getTables returns list of tables in a database
func (v *Verifier) getTables(ctx context.Context, db *sql.DB) ([]string, error) {
	query := `
		SELECT tablename 
		FROM pg_tables 
		WHERE schemaname = 'public'
		ORDER BY tablename
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}

	return tables, rows.Err()
}

// GenerateReport generates a summary report
func (v *Verifier) GenerateReport(results []VerificationResult) string {
	var report string
	report += "\n========================================\n"
	report += "       MIGRATION VERIFICATION REPORT     \n"
	report += "========================================\n\n"

	totalTables := len(results)
	matchedTables := 0
	totalRemoteRows := int64(0)
	totalLocalRows := int64(0)
	errors := 0

	for _, r := range results {
		if r.Error != nil {
			errors++
			report += fmt.Sprintf("✗ %s - ERROR: %s\n", r.Table, r.Error.Error())
		} else if r.Match {
			matchedTables++
			totalRemoteRows += r.RemoteRows
			totalLocalRows += r.LocalRows
			report += fmt.Sprintf("✓ %s - %d rows\n", r.Table, r.LocalRows)
		} else {
			totalRemoteRows += r.RemoteRows
			totalLocalRows += r.LocalRows
			report += fmt.Sprintf("✗ %s - MISMATCH (Remote: %d, Local: %d, Diff: %d)\n",
				r.Table, r.RemoteRows, r.LocalRows, r.RowDiff)
		}
	}

	report += "\n========================================\n"
	report += fmt.Sprintf("Total Tables:    %d\n", totalTables)
	report += fmt.Sprintf("Matched:         %d\n", matchedTables)
	report += fmt.Sprintf("Mismatched:      %d\n", totalTables-matchedTables-errors)
	report += fmt.Sprintf("Errors:          %d\n", errors)
	report += fmt.Sprintf("Total Rows:      %d (Remote) / %d (Local)\n", totalRemoteRows, totalLocalRows)
	report += "========================================\n"

	return report
}
