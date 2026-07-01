// Package dataextract provides window, current, and discovery extraction services.
//
// DJSON (Dynamic JSON) format:
//   {
//     "data_extraction_execution_id": "<exec-id>",
//     "columns": ["col_a", "col_b", ...],
//     "rows": [
//       [val_a1, val_b1, ...],
//       [val_a2, val_b2, ...]
//     ]
//   }
// Column names are derived from pgx.FieldDescriptions() — never hard-coded.
// NULLs become JSON null. Timestamps are UTC ISO 8601 (RFC3339Nano).
// See specs/010-data-api/contracts/api.md for full specification.
package dataextract

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DJSONResponse is the columnar extraction response format.
// Columns lists the field names; Rows contains one slice per row.
// StartAt and EndAt are populated by window extractions; omitted for current extractions.
type DJSONResponse struct {
	ExecutionID string          `json:"data_extraction_execution_id"`
	StartAt     *time.Time      `json:"start_at,omitempty"`
	EndAt       *time.Time      `json:"end_at,omitempty"`
	Columns     []string        `json:"columns"`
	Rows        [][]interface{} `json:"rows"`
}

// Serialise converts pgx.Rows into a DJSONResponse.
// Column names come from FieldDescriptions — they are never hard-coded.
// Sensitive columns (password_hash, key_hash, etc.) are excluded from output.
// NULLs become JSON null. Timestamps are formatted as UTC RFC3339Nano strings.
func Serialise(rows pgx.Rows, execID string) (DJSONResponse, error) {
	fds := rows.FieldDescriptions()

	// Build allowed column indices, excluding sensitive columns.
	allowedIdx := make([]int, 0, len(fds))
	columns := make([]string, 0, len(fds))
	for i, fd := range fds {
		if !IsSensitiveColumn(fd.Name) {
			allowedIdx = append(allowedIdx, i)
			columns = append(columns, fd.Name)
		}
	}

	result := DJSONResponse{
		ExecutionID: execID,
		Columns:     columns,
		Rows:        make([][]interface{}, 0),
	}

	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return DJSONResponse{}, fmt.Errorf("scan row values: %w", err)
		}

		row := make([]interface{}, len(allowedIdx))
		for j, idx := range allowedIdx {
			row[j] = normaliseValue(vals[idx])
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return DJSONResponse{}, fmt.Errorf("iterate rows: %w", err)
	}

	return result, nil
}

// normaliseValue converts pgx native types into JSON-serialisable values.
// Timestamps become UTC ISO 8601 strings; nulls become nil.
func normaliseValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case pgtype.Timestamptz:
		if !val.Valid {
			return nil
		}
		return val.Time.UTC().Format(time.RFC3339Nano)
	case pgtype.Timestamp:
		if !val.Valid {
			return nil
		}
		return val.Time.UTC().Format(time.RFC3339Nano)
	case pgtype.Date:
		if !val.Valid {
			return nil
		}
		return val.Time.UTC().Format("2006-01-02")
	case time.Time:
		return val.UTC().Format(time.RFC3339Nano)
	case pgtype.Text:
		if !val.Valid {
			return nil
		}
		return val.String
	case pgtype.Int4:
		if !val.Valid {
			return nil
		}
		return val.Int32
	case pgtype.Int8:
		if !val.Valid {
			return nil
		}
		return val.Int64
	case pgtype.Float4:
		if !val.Valid {
			return nil
		}
		return val.Float32
	case pgtype.Float8:
		if !val.Valid {
			return nil
		}
		return val.Float64
	case pgtype.Bool:
		if !val.Valid {
			return nil
		}
		return val.Bool
	case pgtype.UUID:
		if !val.Valid {
			return nil
		}
		return fmt.Sprintf("%x-%x-%x-%x-%x",
			val.Bytes[0:4], val.Bytes[4:6], val.Bytes[6:8], val.Bytes[8:10], val.Bytes[10:16])
	case [16]byte:
		// pgx v5 rows.Values() returns UUID columns as [16]byte.
		return fmt.Sprintf("%x-%x-%x-%x-%x",
			val[0:4], val[4:6], val[6:8], val[8:10], val[10:16])
	default:
		return v
	}
}
