package format

import (
	"encoding/csv"
	"io"
)

// csvFormatter writes rows in RFC 4180 CSV format.
type csvFormatter struct {
	w *csv.Writer
}

// newCSVFormatter returns a csvFormatter that writes to w.
func newCSVFormatter(w io.Writer) *csvFormatter {
	return &csvFormatter{w: csv.NewWriter(w)}
}

// WriteHeader writes the column header row.
func (f *csvFormatter) WriteHeader(cols []string) error {
	return f.w.Write(cols)
}

// WriteRow writes a single data row.
func (f *csvFormatter) WriteRow(row []string) error {
	return f.w.Write(row)
}

// Flush flushes any buffered data to the underlying writer.
func (f *csvFormatter) Flush() error {
	f.w.Flush()
	return f.w.Error()
}
