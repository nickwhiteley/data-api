// Package errlog is the single error-logging entry point for the data API.
// All error-level events must pass through Log so future enhancements
// (alerting, aggregation, rate-limiting) only require changes here.
package errlog

import (
	"context"
	"log/slog"
)

// Log records an error at ERROR level. msg names the failed operation.
func Log(ctx context.Context, msg string, err error, attrs ...any) {
	slog.ErrorContext(ctx, msg, append([]any{"error", err}, attrs...)...)
}
