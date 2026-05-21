package format

import (
	"fmt"
	"io"
)

// OutputFormat identifies the desired output file format.
type OutputFormat string

const (
	// FormatCSV produces comma-separated value output.
	FormatCSV OutputFormat = "CSV"
	// FormatTSV produces tab-separated value output (Phase 5).
	FormatTSV OutputFormat = "TSV"
	// FormatPIPE produces pipe-separated value output (Phase 5).
	FormatPIPE OutputFormat = "PIPE"
)

// Formatter writes structured rows to an output file.
type Formatter interface {
	WriteHeader(cols []string) error
	WriteRow(row []string) error
	Flush() error
}

// NewFormatter returns a Formatter for the given OutputFormat writing to w.
func NewFormatter(f OutputFormat, w io.Writer) (Formatter, error) {
	switch f {
	case FormatCSV:
		return newCSVFormatter(w), nil
	case FormatTSV:
		return newTSVFormatter(w), nil
	case FormatPIPE:
		return newPipeFormatter(w), nil
	default:
		return nil, fmt.Errorf("format %q: unknown format", f)
	}
}

// FileExtension returns the file extension (including leading dot) for the given format.
func FileExtension(f OutputFormat) string {
	switch f {
	case FormatTSV:
		return ".tsv"
	case FormatPIPE:
		return ".txt"
	default:
		return ".csv"
	}
}
