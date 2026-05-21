package format

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
)

func TestCSVFormatter(t *testing.T) {
	t.Parallel()

	t.Run("WriteHeader then WriteRow produces valid CSV", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newCSVFormatter(&buf)

		if err := f.WriteHeader([]string{"id", "name", "amount"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"1", "Alice", "100.00"}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}

		r := csv.NewReader(&buf)
		records, err := r.ReadAll()
		if err != nil {
			t.Fatalf("parse CSV: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("records count = %d, want 2", len(records))
		}
		if records[0][0] != "id" || records[0][1] != "name" || records[0][2] != "amount" {
			t.Errorf("header = %v, want [id name amount]", records[0])
		}
		if records[1][0] != "1" || records[1][1] != "Alice" || records[1][2] != "100.00" {
			t.Errorf("row = %v, want [1 Alice 100.00]", records[1])
		}
	})

	t.Run("nil value becomes empty field not NULL", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newCSVFormatter(&buf)

		// Use two columns so the empty value produces "val,\n" rather than
		// a blank line (which csv.Reader skips for single-field records).
		if err := f.WriteHeader([]string{"name", "optional"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		// In practice the caller converts nil → "" before WriteRow; test that
		// writing an empty string produces an empty CSV field (not "NULL").
		if err := f.WriteRow([]string{"Alice", ""}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}

		r := csv.NewReader(&buf)
		records, err := r.ReadAll()
		if err != nil {
			t.Fatalf("parse CSV: %v", err)
		}
		if len(records) < 2 {
			t.Fatalf("expected 2 records (header + row), got %d: %s", len(records), buf.String())
		}
		if records[1][1] != "" {
			t.Errorf("null field = %q, want empty string", records[1][1])
		}
		if strings.Contains(buf.String(), "NULL") {
			t.Errorf("output contains NULL: %s", buf.String())
		}
	})

	t.Run("comma in field is quoted per RFC 4180", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newCSVFormatter(&buf)

		if err := f.WriteHeader([]string{"name"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"Smith, John"}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}

		r := csv.NewReader(&buf)
		records, err := r.ReadAll()
		if err != nil {
			t.Fatalf("parse CSV: %v", err)
		}
		if records[1][0] != "Smith, John" {
			t.Errorf("field = %q, want %q", records[1][0], "Smith, John")
		}
	})

	t.Run("bool true becomes string true", func(t *testing.T) {
		t.Parallel()

		// csvFormatter only deals in []string; the caller converts bools.
		// Verify that the string "true" round-trips correctly.
		var buf bytes.Buffer
		f := newCSVFormatter(&buf)
		if err := f.WriteHeader([]string{"active"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"true"}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}

		r := csv.NewReader(&buf)
		records, err := r.ReadAll()
		if err != nil {
			t.Fatalf("parse CSV: %v", err)
		}
		if records[1][0] != "true" {
			t.Errorf("bool field = %q, want %q", records[1][0], "true")
		}
	})

	t.Run("json.Number large integer exact decimal string", func(t *testing.T) {
		t.Parallel()

		const bigInt = "9999999999999999"
		num := json.Number(bigInt)

		var buf bytes.Buffer
		f := newCSVFormatter(&buf)
		if err := f.WriteHeader([]string{"n"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{num.String()}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}

		r := csv.NewReader(&buf)
		records, err := r.ReadAll()
		if err != nil {
			t.Fatalf("parse CSV: %v", err)
		}
		if records[1][0] != bigInt {
			t.Errorf("large int = %q, want %q", records[1][0], bigInt)
		}
	})

	t.Run("Flush required: content appears after Flush", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newCSVFormatter(&buf)

		if err := f.WriteHeader([]string{"x"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"val"}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}

		// csv.Writer buffers internally; data should appear after Flush.
		// We can't assert buf is empty before Flush (implementation detail),
		// but we assert it is non-empty AFTER Flush.
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		if buf.Len() == 0 {
			t.Error("buffer empty after Flush; expected content")
		}
	})
}

func TestNewFormatter(t *testing.T) {
	t.Parallel()

	t.Run("CSV returns formatter", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		f, err := NewFormatter(FormatCSV, &buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil {
			t.Error("expected non-nil formatter")
		}
	})

	t.Run("TSV returns formatter", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		f, err := NewFormatter(FormatTSV, &buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil {
			t.Error("expected non-nil formatter")
		}
	})

	t.Run("PIPE returns formatter", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		f, err := NewFormatter(FormatPIPE, &buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil {
			t.Error("expected non-nil formatter")
		}
	})

	t.Run("unknown format returns error", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		_, err := NewFormatter("EXCEL", &buf)
		if err == nil {
			t.Fatal("expected error for unknown format, got nil")
		}
	})
}

func TestFileExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format OutputFormat
		want   string
	}{
		{FormatCSV, ".csv"},
		{FormatTSV, ".tsv"},
		{FormatPIPE, ".txt"},
		{"UNKNOWN", ".csv"}, // default fallback
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			t.Parallel()
			got := FileExtension(tt.format)
			if got != tt.want {
				t.Errorf("FileExtension(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestTSVFormatter(t *testing.T) {
	t.Parallel()

	t.Run("fields separated by tab not comma", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newTSVFormatter(&buf)
		if err := f.WriteHeader([]string{"id", "name"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"1", "Alice"}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "\t") {
			t.Errorf("TSV output contains no tab: %q", out)
		}
		if strings.Contains(out, ",") {
			t.Errorf("TSV output contains comma: %q", out)
		}
	})

	t.Run("empty field stays empty not NULL", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newTSVFormatter(&buf)
		if err := f.WriteHeader([]string{"a", "b"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"val", ""}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		if strings.Contains(buf.String(), "NULL") {
			t.Errorf("TSV output contains NULL: %s", buf.String())
		}
	})

	t.Run("comma in field is not quoted", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newTSVFormatter(&buf)
		if err := f.WriteHeader([]string{"name"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"Smith, John"}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		// csv.Writer only quotes when the delimiter appears in the field; comma is not the delimiter here.
		if strings.Contains(buf.String(), `"`) {
			t.Errorf("TSV output quoted a field containing only a comma: %q", buf.String())
		}
	})

	t.Run("tab in field is quoted", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newTSVFormatter(&buf)
		if err := f.WriteHeader([]string{"note"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"line1\tline2"}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		// csv.Writer quotes fields containing the delimiter.
		if !strings.Contains(buf.String(), `"`) {
			t.Errorf("TSV output did not quote field containing tab: %q", buf.String())
		}
	})
}

func TestPipeFormatter(t *testing.T) {
	t.Parallel()

	t.Run("fields separated by pipe not comma", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newPipeFormatter(&buf)
		if err := f.WriteHeader([]string{"id", "name"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"1", "Alice"}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "|") {
			t.Errorf("PIPE output contains no pipe: %q", out)
		}
		if strings.Contains(out, ",") {
			t.Errorf("PIPE output contains comma: %q", out)
		}
	})

	t.Run("empty field stays empty not NULL", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newPipeFormatter(&buf)
		if err := f.WriteHeader([]string{"a", "b"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"val", ""}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		if strings.Contains(buf.String(), "NULL") {
			t.Errorf("PIPE output contains NULL: %s", buf.String())
		}
	})

	t.Run("pipe in field is quoted", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newPipeFormatter(&buf)
		if err := f.WriteHeader([]string{"note"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"a|b"}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		if !strings.Contains(buf.String(), `"`) {
			t.Errorf("PIPE output did not quote field containing pipe: %q", buf.String())
		}
	})

	t.Run("comma in field is not quoted", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		f := newPipeFormatter(&buf)
		if err := f.WriteHeader([]string{"name"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := f.WriteRow([]string{"Smith, John"}); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := f.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		if strings.Contains(buf.String(), `"`) {
			t.Errorf("PIPE output quoted a field containing only a comma: %q", buf.String())
		}
	})
}
