// Package dataextract provides window, current, and discovery extraction services.
package dataextract

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExtractWindowInput holds parameters for a window extraction query.
type ExtractWindowInput struct {
	TenantID   string
	TableName  string // base table name, e.g. "account"; query hits wd.account_log
	RowCount   int
	PageNumber int
	StartAt    time.Time
	EndAt      time.Time
}

// ValidateTable returns ErrNotFound if the _log table does not exist in the wd schema.
// This prevents injection and gives a clean 404 for non-existent tables.
func ValidateTable(ctx context.Context, pool *pgxpool.Pool, tableName string) error {
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(
            SELECT 1 FROM information_schema.tables
            WHERE table_schema = 'wd' AND table_name = $1
        )`, tableName+"_log",
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("validate table %s: %w", tableName, err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

// ValidateBaseTable returns ErrNotFound if wd.{tableName} does not exist.
func ValidateBaseTable(ctx context.Context, pool *pgxpool.Pool, tableName string) error {
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'wd' AND table_name = $1)`,
		tableName,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("validate base table %s: %w", tableName, err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

// ExtractCurrentInput holds parameters for a current-state extraction query.
type ExtractCurrentInput struct {
	TenantID   string
	TableName  string // base table name, e.g. "account"
	RowCount   int
	PageNumber int
}

// ExtractCurrent runs a paginated query against wd.{tableName} (base table, not log).
// Returns non-deleted rows ordered by primary key for consistent pagination.
func ExtractCurrent(ctx context.Context, pool *pgxpool.Pool, input ExtractCurrentInput) (pgx.Rows, error) {
	table := "wd." + input.TableName
	offset := (input.PageNumber - 1) * input.RowCount
	// #nosec G201 — tableName validated against information_schema before this call.
	query := fmt.Sprintf(
		`SELECT * FROM %s WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY %s_id ASC LIMIT $2 OFFSET $3`,
		table, input.TableName,
	)
	rows, err := pool.Query(ctx, query, input.TenantID, input.RowCount, offset)
	if err != nil {
		return nil, fmt.Errorf("extract current %s: %w", input.TableName, err)
	}
	return rows, nil
}

// TableInfo holds a discovered extractable table name and description.
type TableInfo struct {
	TableName   string `json:"table_name"`
	Description string `json:"description"`
}

// DiscoverTables returns all wd schema _log tables that have a corresponding base table
// with a tenant_id column, excluding denied tables and data_extraction_execution_log.
// The tenant_id requirement excludes platform-wide tables (e.g. platform_config,
// data_extraction_deny) that cannot be safely filtered per-tenant.
func DiscoverTables(ctx context.Context, pool *pgxpool.Pool) ([]TableInfo, error) {
	rows, err := pool.Query(ctx, `
		SELECT
		    t.table_name,
		    COALESCE(obj_description(pc.oid, 'pg_class'), '') AS description
		FROM information_schema.tables t
		JOIN information_schema.tables base
		    ON base.table_schema = 'wd'
		   AND base.table_name = regexp_replace(t.table_name, '_log$', '')
		JOIN information_schema.columns tc
		    ON tc.table_schema = 'wd'
		   AND tc.table_name = regexp_replace(t.table_name, '_log$', '')
		   AND tc.column_name = 'tenant_id'
		LEFT JOIN pg_catalog.pg_class pc
		    ON pc.relname = t.table_name
		   AND pc.relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = 'wd')
		WHERE t.table_schema = 'wd'
		  AND t.table_name LIKE '%_log'
		  AND t.table_name != 'data_extraction_execution_log'
		  AND NOT EXISTS (
		      SELECT 1 FROM wd.data_extraction_deny d
		      WHERE d.table_name = regexp_replace(t.table_name, '_log$', '')
		        AND d.deleted_at IS NULL
		  )
		ORDER BY t.table_name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("discover tables: %w", err)
	}
	defer rows.Close()

	tables := make([]TableInfo, 0)
	for rows.Next() {
		var ti TableInfo
		if err := rows.Scan(&ti.TableName, &ti.Description); err != nil {
			return nil, fmt.Errorf("scan table info: %w", err)
		}
		// Strip _log suffix — callers use the base name in API calls.
		ti.TableName = strings.TrimSuffix(ti.TableName, "_log")
		tables = append(tables, ti)
	}
	return tables, rows.Err()
}

// ExtractWindow runs a paginated window query against wd.{tableName}_log.
// Returns pgx.Rows so the handler can stream-serialise via Serialise().
// Caller must close rows.
func ExtractWindow(ctx context.Context, pool *pgxpool.Pool, input ExtractWindowInput) (pgx.Rows, error) {
	table := "wd." + input.TableName + "_log"
	offset := (input.PageNumber - 1) * input.RowCount

	// #nosec G201 — tableName validated against information_schema before this call.
	query := fmt.Sprintf(
		`SELECT * FROM %s
         WHERE tenant_id = $1
           AND modified_at >= $2
           AND modified_at < $3
         ORDER BY modified_at ASC, %s_log_id ASC
         LIMIT $4 OFFSET $5`,
		table, input.TableName,
	)
	rows, err := pool.Query(ctx, query,
		input.TenantID, input.StartAt, input.EndAt,
		input.RowCount, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("extract window %s: %w", input.TableName, err)
	}
	return rows, nil
}
