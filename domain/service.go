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
	Schema     string // PostgreSQL schema name, e.g. "wd" or "core"
	TenantID   string
	TableName  string // base table name, e.g. "account"; query hits {schema}.account_log
	RowCount   int
	PageNumber int
	StartAt    time.Time
	EndAt      time.Time
}

// ValidateTable returns ErrNotFound if the _log table does not exist in the given schema.
// This prevents injection and gives a clean 404 for non-existent tables.
func ValidateTable(ctx context.Context, pool *pgxpool.Pool, schema, tableName string) error {
	var exists bool
	schemaName := strings.TrimSuffix(schema, ".")
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(
            SELECT 1 FROM information_schema.tables
            WHERE table_schema = COALESCE(NULLIF($1, ''), current_schema()) AND table_name = $2
        )`, schemaName, tableName+"_log",
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("validate table %s: %w", tableName, err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

// ValidateBaseTable returns ErrNotFound if {schema}.{tableName} does not exist.
func ValidateBaseTable(ctx context.Context, pool *pgxpool.Pool, schema, tableName string) error {
	var exists bool
	schemaName := strings.TrimSuffix(schema, ".")
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = COALESCE(NULLIF($1, ''), current_schema()) AND table_name = $2)`,
		schemaName, tableName,
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
	Schema     string // PostgreSQL schema name
	TenantID   string
	TableName  string // base table name, e.g. "account"
	RowCount   int
	PageNumber int
}

// ExtractCurrent runs a paginated query against {schema}{tableName} (base table, not log).
// Returns non-deleted rows ordered by primary key for consistent pagination.
// Table name is double-quoted to handle PostgreSQL reserved words (e.g. "user").
func ExtractCurrent(ctx context.Context, pool *pgxpool.Pool, input ExtractCurrentInput) (pgx.Rows, error) {
	offset := (input.PageNumber - 1) * input.RowCount
	// #nosec G201 — tableName validated against information_schema before this call; schema is library-configured.
	query := fmt.Sprintf(
		`SELECT * FROM %s"%s" WHERE deleted_at IS NULL ORDER BY "%s_id" ASC LIMIT $1 OFFSET $2`,
		input.Schema, input.TableName, input.TableName,
	)
	rows, err := pool.Query(ctx, query, input.RowCount, offset)
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

// DiscoverTables returns all {schema} _log tables that have a corresponding base table,
// excluding denied tables and data_extraction_execution_log.
func DiscoverTables(ctx context.Context, pool *pgxpool.Pool, schema string) ([]TableInfo, error) {
	schemaName := strings.TrimSuffix(schema, ".")
	rows, err := pool.Query(ctx, `
		SELECT
		    t.table_name,
		    COALESCE(obj_description(pc.oid, 'pg_class'), '') AS description
		FROM information_schema.tables t
		JOIN information_schema.tables base
		    ON base.table_schema = COALESCE(NULLIF($1, ''), current_schema())
		   AND base.table_name = regexp_replace(t.table_name, '_log$', '')
		LEFT JOIN pg_catalog.pg_class pc
		    ON pc.relname = t.table_name
		   AND pc.relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = COALESCE(NULLIF($1, ''), current_schema()))
		WHERE t.table_schema = COALESCE(NULLIF($1, ''), current_schema())
		  AND t.table_name LIKE '%_log'
		  AND t.table_name != 'data_extraction_execution_log'
		  AND NOT EXISTS (
		      SELECT 1 FROM ` + schema + `data_extraction_deny d
		      WHERE d.table_name = regexp_replace(t.table_name, '_log$', '')
		        AND d.deleted_at IS NULL
		  )
		ORDER BY t.table_name ASC`, schemaName,
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

// ExtractWindow runs a paginated window query against {schema}{tableName}_log.
// Returns pgx.Rows so the handler can stream-serialise via Serialise().
// Table name is double-quoted to handle PostgreSQL reserved words (e.g. "user").
// Caller must close rows.
func ExtractWindow(ctx context.Context, pool *pgxpool.Pool, input ExtractWindowInput) (pgx.Rows, error) {
	offset := (input.PageNumber - 1) * input.RowCount

	// #nosec G201 — tableName validated against information_schema before this call; schema is library-configured.
	query := fmt.Sprintf(
		`SELECT * FROM %s"%s_log"
         WHERE modified_at >= $1
           AND modified_at < $2
         ORDER BY modified_at ASC, "%s_log_id" ASC
         LIMIT $3 OFFSET $4`,
		input.Schema, input.TableName, input.TableName,
	)
	rows, err := pool.Query(ctx, query,
		input.StartAt, input.EndAt,
		input.RowCount, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("extract window %s: %w", input.TableName, err)
	}
	return rows, nil
}
