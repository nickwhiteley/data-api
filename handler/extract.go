// Package handler provides HTTP handlers for the data extraction API.
package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	dataextract "github.com/nickwhiteley/data-api/domain"
	"github.com/nickwhiteley/data-api/pkg/apierror"
	"github.com/nickwhiteley/data-api/pkg/errlog"
	"github.com/nickwhiteley/data-api/pkg/respond"
)

// Config holds library-level configuration for the handler.
type Config struct {
	// Schema is the PostgreSQL schema name for all extraction queries.
	// When empty, table names are unqualified and rely on the session search_path.
	Schema string
	// RequiredScope is the API key scope checked on every extraction request.
	// Defaults to "data_engineer" if empty.
	RequiredScope string
}

// ExtractHandler handles data extraction HTTP requests.
type ExtractHandler struct {
	pool          *pgxpool.Pool
	auth          AuthContext
	cfg           DataConfig
	schema        string
	requiredScope string
}

// NewHandler creates a data extraction handler.
// auth extracts scope/userID/tenantID from the request context using the host application's middleware.
// cfg supplies runtime config values; implementations should read from a live source.
func NewHandler(pool *pgxpool.Pool, auth AuthContext, cfg DataConfig, config Config) *ExtractHandler {
	schema := config.Schema
	if schema != "" {
		schema = schema + "."
	}
	requiredScope := config.RequiredScope
	if requiredScope == "" {
		requiredScope = "data_engineer"
	}
	return &ExtractHandler{pool: pool, auth: auth, cfg: cfg, schema: schema, requiredScope: requiredScope}
}

// Handler returns an http.Handler with all data extraction routes registered.
// Mount it under the desired prefix in the host application's router.
func (h *ExtractHandler) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /data/executions", h.ListExecutions)
	mux.HandleFunc("GET /data/extract", h.DiscoverTables)
	mux.HandleFunc("GET /data/extract/{table}", h.WindowExtract)
	mux.HandleFunc("GET /data/extract/{table}/current", h.CurrentExtract)
	mux.HandleFunc("POST /data/extract/{table}/reset", h.ResetExtraction)
	return mux
}

// WindowExtract handles GET /v1/data/extract/{table}.
// A data engineer (authenticated via API key with the required scope) extracts
// incremental changes from {schema}.{table}_log using a moving cursor.
func (h *ExtractHandler) WindowExtract(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tableName := r.PathValue("table")
	tenantID := h.auth.TenantID(ctx)
	userID := h.auth.UserID(ctx)

	// Step 1: scope check.
	if h.auth.Scope(ctx) != h.requiredScope {
		respond.JSON(w, http.StatusForbidden, apierror.New(apierror.CodeForbidden, h.requiredScope+" scope required"))
		return
	}

	// Step 2: deny check.
	denied, err := dataextract.IsDenied(ctx, h.pool, h.schema, tableName)
	if err != nil {
		errlog.Log(ctx, "check deny list", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}
	if denied {
		respond.JSON(w, http.StatusForbidden, apierror.New(apierror.CodeForbidden, "table is not available for extraction"))
		return
	}

	// Step 3: table existence.
	if err := dataextract.ValidateTable(ctx, h.pool, h.schema, tableName); err != nil {
		if errors.Is(err, dataextract.ErrNotFound) {
			respond.JSON(w, http.StatusNotFound, apierror.New(apierror.CodeNotFound, "table not found"))
			return
		}
		errlog.Log(ctx, "validate table", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}

	// Step 4: parse row_count (optional, default 1000).
	rowCount := 1000
	if rowCountStr := r.URL.Query().Get("row_count"); rowCountStr != "" {
		var parseErr error
		rowCount, parseErr = strconv.Atoi(rowCountStr)
		if parseErr != nil || rowCount <= 0 {
			respond.JSON(w, http.StatusBadRequest, apierror.New(apierror.CodeValidationError, "row_count must be a positive integer"))
			return
		}
		if rowCount > 10000 {
			respond.JSON(w, http.StatusBadRequest, apierror.New(apierror.CodeValidationError, "row_count exceeds maximum of 10000"))
			return
		}
	}

	// Step 5: parse page_number (optional, default 1).
	pageNumber := 1
	if pStr := r.URL.Query().Get("page_number"); pStr != "" {
		var parseErr error
		pageNumber, parseErr = strconv.Atoi(pStr)
		if parseErr != nil || pageNumber < 1 {
			respond.JSON(w, http.StatusBadRequest, apierror.New(apierror.CodeValidationError, "page_number must be a positive integer"))
			return
		}
	}

	// Step 6: determine execution context.
	var execID string
	var startAt, endAt time.Time

	if pageNumber == 1 {
		// Resolve start_at.
		if startStr := r.URL.Query().Get("start_at"); startStr != "" {
			var parseErr error
			startAt, parseErr = parseTimeParam(startStr)
			if parseErr != nil {
				respond.JSON(w, http.StatusBadRequest, apierror.New(apierror.CodeValidationError, fmt.Sprintf("invalid start_at: %v", parseErr)))
				return
			}
		} else {
			var cursorErr error
			startAt, cursorErr = dataextract.CursorFor(ctx, h.pool, h.schema, userID, tableName)
			if cursorErr != nil {
				errlog.Log(ctx, "get cursor", cursorErr)
				respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
				return
			}
		}

		// Resolve end_at.
		if endStr := r.URL.Query().Get("end_at"); endStr != "" {
			var parseErr error
			endAt, parseErr = parseTimeParam(endStr)
			if parseErr != nil {
				respond.JSON(w, http.StatusBadRequest, apierror.New(apierror.CodeValidationError, fmt.Sprintf("invalid end_at: %v", parseErr)))
				return
			}
		} else {
			lag := h.cfg.SafetyLagSeconds()
			endAt = time.Now().UTC().Add(-time.Duration(lag) * time.Second)
		}

		var insertErr error
		execID, insertErr = dataextract.InsertOrReusePending(ctx, h.pool, dataextract.InsertPendingInput{
			Schema:      h.schema,
			TenantID:    tenantID,
			UserID:      userID,
			TableName:   tableName,
			ExtractType: "window",
			StartAt:     startAt,
			EndAt:       endAt,
		})
		if insertErr != nil {
			errlog.Log(ctx, "insert or reuse pending execution", insertErr)
			respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
			return
		}
	} else {
		// page_number > 1: require data_extraction_execution_id.
		execIDParam := r.URL.Query().Get("data_extraction_execution_id")
		if execIDParam == "" {
			respond.JSON(w, http.StatusBadRequest, apierror.New(apierror.CodeValidationError, "data_extraction_execution_id is required for page 2+"))
			return
		}
		exec, execErr := dataextract.GetExecutionByID(ctx, h.pool, h.schema, execIDParam, userID)
		if execErr != nil {
			if errors.Is(execErr, dataextract.ErrNotFound) {
				respond.JSON(w, http.StatusNotFound, apierror.New(apierror.CodeNotFound, "execution not found"))
				return
			}
			errlog.Log(ctx, "get execution by id", execErr)
			respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
			return
		}
		execID = exec.ExecutionID
		startAt = exec.StartAt
		endAt = exec.EndAt
	}

	// Step 7: run extraction.
	rows, err := dataextract.ExtractWindow(ctx, h.pool, dataextract.ExtractWindowInput{
		Schema:     h.schema,
		TenantID:   tenantID,
		TableName:  tableName,
		RowCount:   rowCount,
		PageNumber: pageNumber,
		StartAt:    startAt,
		EndAt:      endAt,
	})
	if err != nil {
		errlog.Log(ctx, "extract window", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}

	// Step 8: serialise.
	result, err := dataextract.Serialise(rows, execID)
	if err != nil {
		errlog.Log(ctx, "serialise extraction rows", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}
	result.StartAt = &startAt
	result.EndAt = &endAt

	// Step 9: status transitions.
	if pageNumber == 1 {
		if err := dataextract.TransitionStarted(ctx, h.pool, h.schema, execID); err != nil && !errors.Is(err, dataextract.ErrNotFound) {
			errlog.Log(ctx, "transition execution to started", err)
		}
	}
	if len(result.Rows) < rowCount {
		if err := dataextract.TransitionCompleted(ctx, h.pool, dataextract.TransitionCompletedInput{
			Schema:    h.schema,
			ExecID:    execID,
			RowCount:  len(result.Rows),
			TimeTaken: 0,
		}); err != nil && !errors.Is(err, dataextract.ErrNotFound) {
			errlog.Log(ctx, "transition execution to completed", err)
		}
	}

	respond.JSON(w, http.StatusOK, result)
}

// CurrentExtract handles GET /v1/data/extract/{table}/current.
func (h *ExtractHandler) CurrentExtract(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tableName := r.PathValue("table")
	tenantID := h.auth.TenantID(ctx)
	userID := h.auth.UserID(ctx)

	// Step 1: scope check.
	if h.auth.Scope(ctx) != h.requiredScope {
		respond.JSON(w, http.StatusForbidden, apierror.New(apierror.CodeForbidden, h.requiredScope+" scope required"))
		return
	}

	// Step 2: deny check.
	denied, err := dataextract.IsDenied(ctx, h.pool, h.schema, tableName)
	if err != nil {
		errlog.Log(ctx, "check deny list", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}
	if denied {
		respond.JSON(w, http.StatusForbidden, apierror.New(apierror.CodeForbidden, "table is not available for extraction"))
		return
	}

	// Step 3: base table existence.
	if err := dataextract.ValidateBaseTable(ctx, h.pool, h.schema, tableName); err != nil {
		if errors.Is(err, dataextract.ErrNotFound) {
			respond.JSON(w, http.StatusNotFound, apierror.New(apierror.CodeNotFound, "table not found"))
			return
		}
		errlog.Log(ctx, "validate base table", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}

	// Step 4: parse row_count (optional, default 1000).
	rowCount := 1000
	if rowCountStr := r.URL.Query().Get("row_count"); rowCountStr != "" {
		var parseErr error
		rowCount, parseErr = strconv.Atoi(rowCountStr)
		if parseErr != nil || rowCount <= 0 {
			respond.JSON(w, http.StatusBadRequest, apierror.New(apierror.CodeValidationError, "row_count must be a positive integer"))
			return
		}
		if rowCount > 10000 {
			respond.JSON(w, http.StatusBadRequest, apierror.New(apierror.CodeValidationError, "row_count exceeds maximum of 10000"))
			return
		}
	}

	// Step 5: parse page_number.
	pageNumber := 1
	if pStr := r.URL.Query().Get("page_number"); pStr != "" {
		var parseErr error
		pageNumber, parseErr = strconv.Atoi(pStr)
		if parseErr != nil || pageNumber < 1 {
			respond.JSON(w, http.StatusBadRequest, apierror.New(apierror.CodeValidationError, "page_number must be a positive integer"))
			return
		}
	}

	var execID string
	now := time.Now().UTC()

	if pageNumber == 1 {
		// Step 6: daily limit check (only on page 1).
		count, countErr := dataextract.CurrentExtractionCount(ctx, h.pool, h.schema, userID, tableName)
		if countErr != nil {
			errlog.Log(ctx, "check daily extraction count", countErr)
			respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
			return
		}
		if count >= 2 {
			respond.JSON(w, http.StatusTooManyRequests, apierror.New(apierror.CodeLimitExceeded, "daily extraction limit of 2 reached for this table"))
			return
		}

		var insertErr error
		execID, insertErr = dataextract.InsertOrReusePending(ctx, h.pool, dataextract.InsertPendingInput{
			Schema:      h.schema,
			TenantID:    tenantID,
			UserID:      userID,
			TableName:   tableName,
			ExtractType: "current",
			StartAt:     now,
			EndAt:       now,
		})
		if insertErr != nil {
			errlog.Log(ctx, "insert or reuse pending current execution", insertErr)
			respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
			return
		}
	} else {
		execIDParam := r.URL.Query().Get("data_extraction_execution_id")
		if execIDParam == "" {
			respond.JSON(w, http.StatusBadRequest, apierror.New(apierror.CodeValidationError, "data_extraction_execution_id is required for page 2+"))
			return
		}
		exec, execErr := dataextract.GetExecutionByID(ctx, h.pool, h.schema, execIDParam, userID)
		if execErr != nil {
			if errors.Is(execErr, dataextract.ErrNotFound) {
				respond.JSON(w, http.StatusNotFound, apierror.New(apierror.CodeNotFound, "execution not found"))
				return
			}
			errlog.Log(ctx, "get current execution by id", execErr)
			respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
			return
		}
		execID = exec.ExecutionID
	}

	rows, err := dataextract.ExtractCurrent(ctx, h.pool, dataextract.ExtractCurrentInput{
		Schema:     h.schema,
		TenantID:   tenantID,
		TableName:  tableName,
		RowCount:   rowCount,
		PageNumber: pageNumber,
	})
	if err != nil {
		errlog.Log(ctx, "extract current", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}

	result, err := dataextract.Serialise(rows, execID)
	if err != nil {
		errlog.Log(ctx, "serialise current extraction rows", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}

	if pageNumber == 1 {
		if err := dataextract.TransitionStarted(ctx, h.pool, h.schema, execID); err != nil && !errors.Is(err, dataextract.ErrNotFound) {
			errlog.Log(ctx, "transition current execution to started", err)
		}
	}
	if len(result.Rows) < rowCount {
		if err := dataextract.TransitionCompleted(ctx, h.pool, dataextract.TransitionCompletedInput{
			Schema:    h.schema,
			ExecID:    execID,
			RowCount:  len(result.Rows),
			TimeTaken: 0,
		}); err != nil && !errors.Is(err, dataextract.ErrNotFound) {
			errlog.Log(ctx, "transition current execution to completed", err)
		}
	}

	respond.JSON(w, http.StatusOK, result)
}

// DiscoverTables handles GET /v1/data/extract.
// Accessible to the required scope (API key) as well as tenant_admin and platform_admin (session auth).
func (h *ExtractHandler) DiscoverTables(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := h.auth.TenantID(ctx)
	userID := h.auth.UserID(ctx)

	scope := h.auth.Scope(ctx)
	if scope != h.requiredScope {
		// Session-based auth — check role in DB.
		var role string
		// #nosec G201 — schema is library-configured, not user input
		err := h.pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT role FROM %suser_org
			 WHERE user_id = $1 AND tenant_id = $2 AND deleted_at IS NULL
			   AND role IN ('data_engineer', 'tenant_admin', 'platform_admin')
			 LIMIT 1`, h.schema),
			userID, tenantID,
		).Scan(&role)
		if err != nil {
			respond.JSON(w, http.StatusForbidden, apierror.New(apierror.CodeForbidden, "access denied"))
			return
		}
	}

	tables, err := dataextract.DiscoverTables(ctx, h.pool, h.schema)
	if err != nil {
		errlog.Log(ctx, "discover tables", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"tables": tables})
}

// ResetExtraction handles POST /v1/data/extract/{table}/reset.
func (h *ExtractHandler) ResetExtraction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tableName := r.PathValue("table")
	tenantID := h.auth.TenantID(ctx)
	userID := h.auth.UserID(ctx)

	if h.auth.Scope(ctx) != h.requiredScope {
		respond.JSON(w, http.StatusForbidden, apierror.New(apierror.CodeForbidden, h.requiredScope+" scope required"))
		return
	}

	denied, err := dataextract.IsDenied(ctx, h.pool, h.schema, tableName)
	if err != nil {
		errlog.Log(ctx, "check deny list for reset", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}
	if denied {
		respond.JSON(w, http.StatusForbidden, apierror.New(apierror.CodeForbidden, "table is not available for extraction"))
		return
	}

	if err := dataextract.ValidateTable(ctx, h.pool, h.schema, tableName); err != nil {
		if errors.Is(err, dataextract.ErrNotFound) {
			respond.JSON(w, http.StatusNotFound, apierror.New(apierror.CodeNotFound, "table not found"))
			return
		}
		errlog.Log(ctx, "validate table for reset", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}

	// Parse optional timestamp; default to 2000-01-01.
	endAt := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if tsStr := r.URL.Query().Get("timestamp"); tsStr != "" {
		t, parseErr := parseTimeParam(tsStr)
		if parseErr != nil {
			respond.JSON(w, http.StatusBadRequest, apierror.New(apierror.CodeValidationError, fmt.Sprintf("invalid timestamp: %v", parseErr)))
			return
		}
		endAt = t
	}

	execID, err := dataextract.InsertReset(ctx, h.pool, dataextract.InsertResetInput{
		Schema:    h.schema,
		TenantID:  tenantID,
		UserID:    userID,
		TableName: tableName,
		EndAt:     endAt,
	})
	if err != nil {
		errlog.Log(ctx, "insert reset", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}

	respond.JSON(w, http.StatusCreated, map[string]any{
		"data_extraction_execution_id": execID,
		"table_name":                   tableName,
		"end_at":                       endAt.Format(time.RFC3339),
	})
}

// ListExecutions handles GET /v1/data/executions.
// Role-based scoping: data_engineer sees own; tenant_admin sees tenant; platform_admin sees all.
func (h *ExtractHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := h.auth.TenantID(ctx)
	userID := h.auth.UserID(ctx)
	scope := h.auth.Scope(ctx)

	var role string
	if scope == "" {
		// Session auth — look up role from DB.
		// #nosec G201 — schema is library-configured, not user input
		_ = h.pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT role FROM %suser_org
			 WHERE user_id = $1 AND tenant_id = $2 AND deleted_at IS NULL
			 LIMIT 1`, h.schema),
			userID, tenantID,
		).Scan(&role)
	} else {
		role = scope
	}

	input := dataextract.ListExecutionsInput{
		Schema:  h.schema,
		TenantID: tenantID,
		Page:    1,
		PerPage: 20,
	}

	switch role {
	case "data_engineer":
		input.UserID = userID
	case "tenant_admin":
		// sees all users in tenant; no user filter unless explicitly provided
	case "platform_admin":
		if t := r.URL.Query().Get("tenant_id"); t != "" {
			input.TenantID = t
		}
	default:
		respond.JSON(w, http.StatusForbidden, apierror.New(apierror.CodeForbidden, "access denied"))
		return
	}

	if uid := r.URL.Query().Get("user_id"); uid != "" {
		if role == "tenant_admin" || role == "platform_admin" {
			input.UserID = uid
		}
	}
	if s := r.URL.Query().Get("start_date"); s != "" {
		if t, err := parseTimeParam(s); err == nil {
			input.StartDate = t
		}
	}
	if s := r.URL.Query().Get("end_date"); s != "" {
		if t, err := parseTimeParam(s); err == nil {
			input.EndDate = t
		}
	}
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			input.Page = n
		}
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if n, err := strconv.Atoi(pp); err == nil && n > 0 && n <= 100 {
			input.PerPage = n
		}
	}

	records, total, err := dataextract.ListExecutions(ctx, h.pool, input)
	if err != nil {
		errlog.Log(ctx, "list executions", err)
		respond.JSON(w, http.StatusInternalServerError, apierror.New(apierror.CodeInternalError, "internal error"))
		return
	}

	respond.JSON(w, http.StatusOK, map[string]any{
		"data":        records,
		"total":       total,
		"page":        input.Page,
		"per_page":    input.PerPage,
		"total_pages": (total + input.PerPage - 1) / input.PerPage,
	})
}

// parseTimeParam parses a time string as RFC3339, then as Unix epoch seconds.
func parseTimeParam(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if epoch, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(epoch, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q: expected RFC3339 or Unix epoch", s)
}
