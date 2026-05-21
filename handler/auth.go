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
	// TenantID returns the tenant UUID string for the authenticated request.
	TenantID(ctx context.Context) string
}

// DataConfig supplies runtime configuration values to the handler.
// Implementations should read from a live config source so changes take effect
// without a restart.
type DataConfig interface {
	// SafetyLagSeconds is the number of seconds subtracted from now() when
	// computing a default end_at for window extractions. Prevents partial writes.
	SafetyLagSeconds() int
}
