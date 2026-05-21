// Package apierror provides standardised API error types and codes.
package apierror

import "fmt"

// Code is a stable, machine-readable error identifier.
type Code string

const (
	CodeUnauthenticated Code = "UNAUTHENTICATED"
	CodeForbidden       Code = "FORBIDDEN"
	CodeNotFound        Code = "NOT_FOUND"
	CodeValidationError Code = "VALIDATION_ERROR"
	CodeLimitExceeded   Code = "LIMIT_EXCEEDED"
	CodeConflict        Code = "CONFLICT"
	CodeInternalError   Code = "INTERNAL_ERROR"
)

// Error is the standard JSON error envelope returned by the data API.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// New creates an Error with the given code and message.
func New(code Code, message string) Error {
	return Error{Code: code, Message: message}
}

// Newf creates an Error with a formatted message.
func Newf(code Code, format string, args ...any) Error {
	return Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
