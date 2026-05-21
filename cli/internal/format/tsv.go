package format

import (
	"encoding/csv"
	"io"
)

type tsvFormatter struct{ w *csv.Writer }

func newTSVFormatter(w io.Writer) *tsvFormatter {
	cw := csv.NewWriter(w)
	cw.Comma = '\t'
	return &tsvFormatter{w: cw}
}

func (f *tsvFormatter) WriteHeader(cols []string) error { return f.w.Write(cols) }
func (f *tsvFormatter) WriteRow(row []string) error     { return f.w.Write(row) }
func (f *tsvFormatter) Flush() error                    { f.w.Flush(); return f.w.Error() }
