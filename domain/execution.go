package dataextract

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// InsertPendingInput holds params for inserting a pending execution row.
type InsertPendingInput struct {
	TenantID    string
	UserID      string
	TableName   string
	ExtractType string // "window" or "current"
	StartAt     time.Time
	EndAt       time.Time
}

// InsertOrReusePending inserts a pending execution row or reuses an existing one.
// Uses an advisory lock to ensure idempotency under concurrent page-1 retries.
// Returns execution ID (never empty on nil error).
func InsertOrReusePending(ctx context.Context, pool *pgxpool.Pool, input InsertPendingInput) (string, error) {
	// Hash the lock key from user_id + table_name.
	lockKey := fmt.Sprintf("%s:%s", input.UserID, input.TableName)

	// Use a transaction so pg_advisory_xact_lock is scoped correctly.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin insert pending tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := setUserCtx(ctx, tx, input.UserID); err != nil {
		return "", fmt.Errorf("set user context: %w", err)
	}

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey); err != nil {
		return "", fmt.Errorf("acquire advisory lock: %w", err)
	}

	// 1. Try to find an existing pending row first (idempotent retry).
	var execID string
	err = tx.QueryRow(ctx, `
		SELECT data_extraction_execution_id
		FROM wd.data_extraction_execution
		WHERE user_id = $1 AND table_name = $2 AND status = 'pending'
		LIMIT 1`,
		input.UserID, input.TableName,
	).Scan(&execID)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("commit reuse pending: %w", err)
		}
		return execID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("fetch pending execution: %w", err)
	}

	// 2. No pending row — insert a new one.
	if err := tx.QueryRow(ctx, `
		INSERT INTO wd.data_extraction_execution
			(tenant_id, user_id, table_name, extract_type, status, start_at, end_at)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6)
		RETURNING data_extraction_execution_id`,
		input.TenantID, input.UserID, input.TableName, input.ExtractType,
		input.StartAt, input.EndAt,
	).Scan(&execID); err != nil {
		return "", fmt.Errorf("insert pending execution: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit insert pending: %w", err)
	}
	return execID, nil
}

// TransitionStarted moves a pending execution to started.
func TransitionStarted(ctx context.Context, pool *pgxpool.Pool, execID string) error {
	tag, err := pool.Exec(ctx, `
		UPDATE wd.data_extraction_execution
		SET status = 'started', updated_at = clock_timestamp()
		WHERE data_extraction_execution_id = $1 AND status = 'pending'`,
		execID,
	)
	if err != nil {
		return fmt.Errorf("transition to started: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("transition to started: %w", ErrNotFound)
	}

	return nil
}

// TransitionCompletedInput holds params for completing an execution.
type TransitionCompletedInput struct {
	ExecID    string
	RowCount  int
	TimeTaken int // milliseconds
}

// TransitionCompleted moves a started/pending execution to completed.
func TransitionCompleted(ctx context.Context, pool *pgxpool.Pool, input TransitionCompletedInput) error {
	tag, err := pool.Exec(ctx, `
		UPDATE wd.data_extraction_execution
		SET status = 'completed',
		    row_count = $2,
		    execution_time_taken = $3,
		    updated_at = clock_timestamp()
		WHERE data_extraction_execution_id = $1 AND status IN ('started', 'pending')`,
		input.ExecID, input.RowCount, input.TimeTaken,
	)
	if err != nil {
		return fmt.Errorf("transition to completed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("transition to completed: %w", ErrNotFound)
	}

	return nil
}

// InsertResetInput holds params for inserting a reset row.
type InsertResetInput struct {
	TenantID  string
	UserID    string
	TableName string
	EndAt     time.Time
}

// InsertReset inserts a reset cursor row. Returns the new execution ID.
func InsertReset(ctx context.Context, pool *pgxpool.Pool, input InsertResetInput) (string, error) {
	var execID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO wd.data_extraction_execution
			(tenant_id, user_id, table_name, extract_type, status, start_at, end_at)
		VALUES ($1, $2, $3, 'window', 'reset', clock_timestamp(), $4)
		RETURNING data_extraction_execution_id`,
		input.TenantID, input.UserID, input.TableName, input.EndAt,
	).Scan(&execID); err != nil {
		return "", fmt.Errorf("insert reset execution: %w", err)
	}

	return execID, nil
}

// CursorFor returns the most recent end_at for the user+table (completed or reset rows).
// Returns 2000-01-01 00:00:00 UTC if no row found.
func CursorFor(ctx context.Context, pool *pgxpool.Pool, userID, tableName string) (time.Time, error) {
	var endAt time.Time
	err := pool.QueryRow(ctx, `
		SELECT end_at
		FROM wd.data_extraction_execution
		WHERE user_id = $1 AND table_name = $2
		  AND status IN ('completed', 'reset')
		  AND deleted_at IS NULL
		ORDER BY inserted_at DESC
		LIMIT 1`,
		userID, tableName,
	).Scan(&endAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), nil
		}
		return time.Time{}, fmt.Errorf("cursor for %s/%s: %w", userID, tableName, err)
	}
	return endAt, nil
}

// CurrentExtractionCount counts started+completed current extractions today for user+table.
func CurrentExtractionCount(ctx context.Context, pool *pgxpool.Pool, userID, tableName string) (int, error) {
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM wd.data_extraction_execution
		WHERE user_id = $1 AND table_name = $2
		  AND extract_type = 'current'
		  AND status IN ('started', 'completed')
		  AND start_at::date = CURRENT_DATE
		  AND deleted_at IS NULL`,
		userID, tableName,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("current extraction count: %w", err)
	}
	return count, nil
}

// Execution holds the fields of a data_extraction_execution row.
type Execution struct {
	ExecutionID string
	TenantID    string
	UserID      string
	TableName   string
	ExtractType string
	Status      string
	StartAt     time.Time
	EndAt       time.Time
	RowCount    int
	TimeTaken   int
}

// GetExecutionByID returns the execution record for the given ID scoped to the user.
// Returns ErrNotFound if not found or the row belongs to a different user.
func GetExecutionByID(ctx context.Context, pool *pgxpool.Pool, execID, userID string) (Execution, error) {
	var e Execution
	err := pool.QueryRow(ctx, `
		SELECT data_extraction_execution_id, tenant_id, user_id, table_name,
		       extract_type, status, start_at, end_at, row_count, execution_time_taken
		FROM wd.data_extraction_execution
		WHERE data_extraction_execution_id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		execID, userID,
	).Scan(
		&e.ExecutionID, &e.TenantID, &e.UserID, &e.TableName,
		&e.ExtractType, &e.Status, &e.StartAt, &e.EndAt, &e.RowCount, &e.TimeTaken,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Execution{}, ErrNotFound
		}
		return Execution{}, fmt.Errorf("get execution by id: %w", err)
	}
	return e, nil
}

// ExecutionRecord is a flat DTO for the executions list endpoint.
type ExecutionRecord struct {
	ExecutionID       string    `json:"data_extraction_execution_id"`
	TenantID          string    `json:"tenant_id"`
	UserID            string    `json:"user_id"`
	TableName         string    `json:"table_name"`
	ExtractType       string    `json:"extract_type"`
	Status            string    `json:"status"`
	StartAt           time.Time `json:"start_at"`
	EndAt             time.Time `json:"end_at"`
	RowCount          int       `json:"row_count"`
	ExecutionTimeTaken int      `json:"execution_time_taken"`
	InsertedAt        time.Time `json:"inserted_at"`
}

// ListExecutionsInput holds filter and pagination params.
type ListExecutionsInput struct {
	TenantID  string
	UserID    string    // optional filter
	StartDate time.Time // optional filter
	EndDate   time.Time // optional filter
	Page      int
	PerPage   int
}

// ListExecutions returns paginated execution records for the given tenant,
// optionally filtered by user and date range. Reset rows are included
// so the cursor history is complete, but callers may choose to hide them
// in UI presentations.
func ListExecutions(ctx context.Context, pool *pgxpool.Pool, input ListExecutionsInput) ([]ExecutionRecord, int, error) {
	if input.Page < 1 {
		input.Page = 1
	}
	if input.PerPage < 1 {
		input.PerPage = 20
	}
	if input.PerPage > 100 {
		input.PerPage = 100
	}

	// Build WHERE clause dynamically.
	args := []any{input.TenantID}
	where := "WHERE tenant_id = $1 AND deleted_at IS NULL"
	argIdx := 1

	if input.UserID != "" {
		argIdx++
		args = append(args, input.UserID)
		where += fmt.Sprintf(" AND user_id = $%d", argIdx)
	}
	if !input.StartDate.IsZero() {
		argIdx++
		args = append(args, input.StartDate)
		where += fmt.Sprintf(" AND start_at >= $%d", argIdx)
	}
	if !input.EndDate.IsZero() {
		argIdx++
		args = append(args, input.EndDate)
		where += fmt.Sprintf(" AND end_at <= $%d", argIdx)
	}

	// Count total.
	var total int
	countQuery := "SELECT COUNT(*) FROM wd.data_extraction_execution " + where
	if err := pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count executions: %w", err)
	}

	// Fetch page.
	argIdx++
	limitArg := argIdx
	argIdx++
	offsetArg := argIdx
	args = append(args, input.PerPage, (input.Page-1)*input.PerPage)

	query := fmt.Sprintf(
		`SELECT data_extraction_execution_id, tenant_id, user_id, table_name,
		        extract_type, status, start_at, end_at, row_count, execution_time_taken,
		        inserted_at
		 FROM wd.data_extraction_execution
		 %s
		 ORDER BY inserted_at DESC
		 LIMIT $%d OFFSET $%d`,
		where, limitArg, offsetArg,
	)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()

	results := make([]ExecutionRecord, 0, input.PerPage)
	for rows.Next() {
		var rec ExecutionRecord
		if err := rows.Scan(
			&rec.ExecutionID, &rec.TenantID, &rec.UserID, &rec.TableName,
			&rec.ExtractType, &rec.Status, &rec.StartAt, &rec.EndAt,
			&rec.RowCount, &rec.ExecutionTimeTaken, &rec.InsertedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan execution record: %w", err)
		}
		results = append(results, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate executions: %w", err)
	}

	return results, total, nil
}

// setUserCtx stores userID as a transaction-local PostgreSQL session variable
// so shadow-log triggers can record modified_by (DB-2).
// The variable is cleared automatically on commit or rollback.
func setUserCtx(ctx context.Context, tx pgx.Tx, userID string) error {
	if userID == "" {
		return fmt.Errorf("setUserCtx: userID must not be empty (DB-2: modified_by cannot be NULL)")
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_user_id', $1, true)", userID); err != nil {
		return fmt.Errorf("set user context: %w", err)
	}
	return nil
}
