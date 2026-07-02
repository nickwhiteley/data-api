package dataextract

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrDenyConflict is returned when attempting to add a table already on the active deny list.
var ErrDenyConflict = errors.New("table already on deny list")

// hardBlockedTables are tables that must never be extractable regardless of deny list state.
// These contain credentials, keys, session data, or sensitive platform/tenant configuration.
var hardBlockedTables = map[string]bool{
	"user_credential":    true,
	"session":            true,
	"api_key":            true,
	"platform_config":    true,
	"tenant_auth_method": true,
	"login_attempt":      true,
	"rate_limit_bucket":  true,
	"sso_state":          true,
	"password_reset":     true,
}

// sensitiveSuffixes are column name suffixes that indicate credential/secret data.
// Pattern-based matching ensures future columns with these suffixes are also redacted
// without requiring an explicit allowlist update.
var sensitiveSuffixes = []string{"_hash", "_secret", "_token", "_key", "_verifier", "_password"}

// normalizeTableName strips any _log suffix to get the canonical base table name.
func normalizeTableName(tableName string) string {
	return strings.TrimSuffix(tableName, "_log")
}

// IsHardBlocked returns true if the table is on the permanent block list.
func IsHardBlocked(tableName string) bool {
	return hardBlockedTables[normalizeTableName(tableName)]
}

// IsSensitiveColumn returns true if the column should be redacted from extracts.
// Matches any column whose name ends with a sensitive suffix.
func IsSensitiveColumn(columnName string) bool {
	for _, suffix := range sensitiveSuffixes {
		if strings.HasSuffix(columnName, suffix) {
			return true
		}
	}
	return false
}

// DenyEntry represents an active deny list row.
type DenyEntry struct {
	DenyID     string    `json:"deny_id"`
	TableName  string    `json:"table_name"`
	InsertedBy string    `json:"inserted_by"`
	InsertedAt time.Time `json:"inserted_at"`
}

// IsDenied checks if a table is on the active deny list or the hard-blocked list.
// tableName may include the _log suffix — it is normalised to the base name before the check.
func IsDenied(ctx context.Context, pool *pgxpool.Pool, schema, tableName string) (bool, error) {
	base := normalizeTableName(tableName)
	if IsHardBlocked(base) {
		return true, nil
	}
	denyTable := schema + "data_extraction_deny"
	var exists bool
	// #nosec G201 — schema is library-configured, not user input
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS(
			SELECT 1 FROM %s
			WHERE table_name = $1 AND deleted_at IS NULL
		)`, denyTable), base,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check deny list: %w", err)
	}
	return exists, nil
}

// ListDenied returns all active deny entries.
func ListDenied(ctx context.Context, pool *pgxpool.Pool, schema string) ([]DenyEntry, error) {
	denyTable := schema + "data_extraction_deny"
	// #nosec G201 — schema is library-configured, not user input
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT data_extraction_deny_id, table_name, inserted_by, inserted_at
		FROM %s
		WHERE deleted_at IS NULL
		ORDER BY table_name ASC`, denyTable),
	)
	if err != nil {
		return nil, fmt.Errorf("list denied tables: %w", err)
	}
	defer rows.Close()

	entries := make([]DenyEntry, 0)
	for rows.Next() {
		var e DenyEntry
		if err := rows.Scan(&e.DenyID, &e.TableName, &e.InsertedBy, &e.InsertedAt); err != nil {
			return nil, fmt.Errorf("scan deny entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deny entries: %w", err)
	}
	return entries, nil
}

// AddDenyInput holds params for adding a deny entry.
type AddDenyInput struct {
	Schema     string // PostgreSQL schema name
	TableName  string
	InsertedBy string // user_id of the platform admin
}

// AddDeny adds a table to the deny list. Returns ErrDenyConflict if already active.
// TableName is normalized to the base name (strips _log suffix) before storing.
func AddDeny(ctx context.Context, pool *pgxpool.Pool, input AddDenyInput) (DenyEntry, error) {
	input.TableName = normalizeTableName(input.TableName)
	denyTable := input.Schema + "data_extraction_deny"
	var e DenyEntry
	// #nosec G201 — schema is library-configured, not user input
	err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s (table_name, inserted_by)
		VALUES ($1, $2)
		RETURNING data_extraction_deny_id, table_name, inserted_by, inserted_at`, denyTable),
		input.TableName, input.InsertedBy,
	).Scan(&e.DenyID, &e.TableName, &e.InsertedBy, &e.InsertedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return DenyEntry{}, ErrDenyConflict
		}
		return DenyEntry{}, fmt.Errorf("add deny entry: %w", err)
	}

	return e, nil
}

// RemoveDeny soft-deletes the deny entry for the given table. Returns ErrNotFound if not active.
// tableName is normalized to the base name (strips _log suffix) before lookup.
func RemoveDeny(ctx context.Context, pool *pgxpool.Pool, schema, tableName string) error {
	tableName = normalizeTableName(tableName)
	denyTable := schema + "data_extraction_deny"
	var denyID string
	// #nosec G201 — schema is library-configured, not user input
	err := pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s
		SET deleted_at = clock_timestamp()
		WHERE table_name = $1 AND deleted_at IS NULL
		RETURNING data_extraction_deny_id`, denyTable),
		tableName,
	).Scan(&denyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("remove deny entry: %w", err)
	}

	return nil
}

// isUniqueViolation returns true when err is a PostgreSQL unique_violation (code 23505).
func isUniqueViolation(err error) bool {
	type pgErr interface {
		SQLState() string
	}
	var pe pgErr
	if errors.As(err, &pe) {
		return pe.SQLState() == "23505"
	}
	return false
}
