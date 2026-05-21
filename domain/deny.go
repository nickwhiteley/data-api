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
func IsDenied(ctx context.Context, pool *pgxpool.Pool, tableName string) (bool, error) {
	base := normalizeTableName(tableName)
	if IsHardBlocked(base) {
		return true, nil
	}
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM wd.data_extraction_deny
			WHERE table_name = $1 AND deleted_at IS NULL
		)`, base,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check deny list: %w", err)
	}
	return exists, nil
}

// ListDenied returns all active deny entries.
func ListDenied(ctx context.Context, pool *pgxpool.Pool) ([]DenyEntry, error) {
	rows, err := pool.Query(ctx, `
		SELECT data_extraction_deny_id, table_name, inserted_by, inserted_at
		FROM wd.data_extraction_deny
		WHERE deleted_at IS NULL
		ORDER BY table_name ASC`,
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
	TableName  string
	InsertedBy string // user_id of the platform admin
}

// AddDeny adds a table to the deny list. Returns ErrDenyConflict if already active.
func AddDeny(ctx context.Context, pool *pgxpool.Pool, input AddDenyInput) (DenyEntry, error) {
	var e DenyEntry
	err := pool.QueryRow(ctx, `
		INSERT INTO wd.data_extraction_deny (table_name, inserted_by)
		VALUES ($1, $2)
		RETURNING data_extraction_deny_id, table_name, inserted_by, inserted_at`,
		input.TableName, input.InsertedBy,
	).Scan(&e.DenyID, &e.TableName, &e.InsertedBy, &e.InsertedAt)
	if err != nil {
		// Unique index violation — table already on active deny list.
		if isUniqueViolation(err) {
			return DenyEntry{}, ErrDenyConflict
		}
		return DenyEntry{}, fmt.Errorf("add deny entry: %w", err)
	}

	return e, nil
}

// RemoveDeny soft-deletes the deny entry for the given table. Returns ErrNotFound if not active.
func RemoveDeny(ctx context.Context, pool *pgxpool.Pool, tableName string) error {
	var denyID string
	err := pool.QueryRow(ctx, `
		UPDATE wd.data_extraction_deny
		SET deleted_at = clock_timestamp()
		WHERE table_name = $1 AND deleted_at IS NULL
		RETURNING data_extraction_deny_id`,
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
	// pgx wraps the PgError; check the SQLState code.
	type pgErr interface {
		SQLState() string
	}
	var pe pgErr
	if errors.As(err, &pe) {
		return pe.SQLState() == "23505"
	}
	return false
}
