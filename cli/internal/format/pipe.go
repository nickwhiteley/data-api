package format

import (
	"encoding/csv"
	"io"
)

type pipeFormatter struct{ w *csv.Writer }

func newPipeFormatter(w io.Writer) *pipeFormatter {
	cw := csv.NewWriter(w)
	cw.Comma = '|'
	return &pipeFormatter{w: cw}
}

func (f *pipeFormatter) WriteHeader(cols []string) error { return f.w.Write(cols) }
func (f *pipeFormatter) WriteRow(row []string) error     { return f.w.Write(row) }
func (f *pipeFormatter) Flush() error                    { f.w.Flush(); return f.w.Error() }
