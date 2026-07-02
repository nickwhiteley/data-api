// Package handler provides HTTP handlers for the data extraction API.
package handler

import "context"

// AuthContext extracts authentication and authorisation values from a request context.
// Implement this interface in the host application using your middleware's context keys.
type AuthContext interface {
	// Scope returns the API key scope (e.g. "data_engineer"). Returns empty string if unset.
	Scope(ctx context.Context) string
	// UserID returns the authenticated user's UUID string.
	UserID(ctx context.Context) string
	// TenantID returns the tenant UUID string used for audit records (execution log rows).
	// Must return a non-empty value; it is stored as NOT NULL in data_extraction_execution.
	TenantID(ctx context.Context) string
	// QueryTenantID returns the tenant UUID to use in data-filtering WHERE clauses.
	// Return a non-empty UUID to enforce per-tenant row isolation (multi-tenant deployments).
	// Return "" to skip the tenant_id filter entirely (single-tenant deployments where
	// business tables have no tenant_id column).
	QueryTenantID(ctx context.Context) string
}

// DataConfig supplies runtime configuration values to the handler.
// Implementations should read from a live config source so changes take effect
// without a restart.
type DataConfig interface {
	// SafetyLagSeconds is the number of seconds subtracted from now() when
	// computing a default end_at for window extractions. Prevents partial writes.
	SafetyLagSeconds() int
}
