package output

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nickwhiteley/data-api/cli/internal/format"
	"github.com/nickwhiteley/data-api/cli/internal/partition"
)

// newTestManager returns an OutputManager writing to dir with a fixed start time.
func newTestManager(dir string, startedAt time.Time) *OutputManager {
	return New(&LocalWriter{}, "account", startedAt, format.FormatCSV, nil, dir)
}

func TestOutputManager(t *testing.T) {
	t.Parallel()

	t.Run("file created under correct path formula", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		startedAt := time.Date(2026, 5, 18, 14, 30, 5, 0, time.UTC)
		mgr := newTestManager(dir, startedAt)

		if err := mgr.WriteHeader([]string{"id", "name"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := mgr.WriteRow([]string{"1", "Alice"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		files := mgr.Files()
		if len(files) != 1 {
			t.Fatalf("Files() len = %d, want 1", len(files))
		}
		wantName := "account-20260518-143005.csv"
		wantPath := filepath.Join(dir, wantName)
		if files[0] != wantPath {
			t.Errorf("path = %q, want %q", files[0], wantPath)
		}
		if _, err := os.Stat(files[0]); err != nil {
			t.Errorf("file does not exist: %v", err)
		}
	})

	t.Run("header row present in output file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		startedAt := time.Now()
		mgr := newTestManager(dir, startedAt)

		if err := mgr.WriteHeader([]string{"col_a", "col_b"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := mgr.WriteRow([]string{"v1", "v2"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		files := mgr.Files()
		if len(files) == 0 {
			t.Fatal("no files written")
		}

		f, err := os.Open(files[0])
		if err != nil {
			t.Fatalf("open output file: %v", err)
		}
		t.Cleanup(func() { _ = f.Close() })

		records, err := csv.NewReader(f).ReadAll()
		if err != nil {
			t.Fatalf("parse CSV: %v", err)
		}
		if len(records) < 1 {
			t.Fatal("no records in output file")
		}
		if records[0][0] != "col_a" || records[0][1] != "col_b" {
			t.Errorf("header = %v, want [col_a col_b]", records[0])
		}
	})

	t.Run("WriteRow after WriteHeader produces correct CSV", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		mgr := newTestManager(dir, time.Now())

		if err := mgr.WriteHeader([]string{"id", "value"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := mgr.WriteRow([]string{"42", "hello"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.WriteRow([]string{"99", "world"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		files := mgr.Files()
		f, err := os.Open(files[0])
		if err != nil {
			t.Fatalf("open output file: %v", err)
		}
		t.Cleanup(func() { _ = f.Close() })

		records, err := csv.NewReader(f).ReadAll()
		if err != nil {
			t.Fatalf("parse CSV: %v", err)
		}
		// header + 2 data rows
		if len(records) != 3 {
			t.Fatalf("records count = %d, want 3", len(records))
		}
		if records[1][0] != "42" || records[1][1] != "hello" {
			t.Errorf("row 1 = %v, want [42 hello]", records[1])
		}
		if records[2][0] != "99" || records[2][1] != "world" {
			t.Errorf("row 2 = %v, want [99 world]", records[2])
		}
	})

	t.Run("files from previous runs are preserved", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		// Simulate a file written by a previous run.
		previous := filepath.Join(dir, "account-20240101-000000.csv")
		if err := os.WriteFile(previous, []byte("previous run data"), 0o644); err != nil {
			t.Fatalf("create previous file: %v", err)
		}

		mgr := newTestManager(dir, time.Now())
		if err := mgr.WriteHeader([]string{"id"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := mgr.WriteRow([]string{"1"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		// Previous run's file must still exist.
		if _, err := os.Stat(previous); err != nil {
			t.Errorf("file from previous run was deleted: %s", previous)
		}

		// New run produced its own file.
		files := mgr.Files()
		if len(files) != 1 {
			t.Fatalf("Files() = %v, want 1 file", files)
		}
	})

	t.Run("Close flushes buffered content", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		mgr := newTestManager(dir, time.Now())

		if err := mgr.WriteHeader([]string{"x"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := mgr.WriteRow([]string{"data-value"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		files := mgr.Files()
		if len(files) == 0 {
			t.Fatal("no files written")
		}
		info, err := os.Stat(files[0])
		if err != nil {
			t.Fatalf("stat output file: %v", err)
		}
		if info.Size() == 0 {
			t.Error("file size is 0 after Close; buffered content was not flushed")
		}
	})

	t.Run("Files returns the written path", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		startedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		mgr := newTestManager(dir, startedAt)

		if err := mgr.WriteHeader([]string{"id"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := mgr.WriteRow([]string{"1"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		files := mgr.Files()
		if len(files) != 1 {
			t.Fatalf("Files() len = %d, want 1", len(files))
		}
		if !strings.HasPrefix(files[0], dir) {
			t.Errorf("file path %q does not start with dir %q", files[0], dir)
		}
		if !strings.HasSuffix(files[0], ".csv") {
			t.Errorf("file path %q does not end with .csv", files[0])
		}
	})

	t.Run("Close with no rows written is a no-op", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		mgr := newTestManager(dir, time.Now())
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close on empty manager: %v", err)
		}
		if len(mgr.Files()) != 0 {
			t.Errorf("Files() = %v, want empty", mgr.Files())
		}
	})

	t.Run("TSV format produces .tsv file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		mgr := New(&LocalWriter{}, "account", time.Now(), format.FormatTSV, nil, dir)

		if err := mgr.WriteHeader([]string{"id", "name"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := mgr.WriteRow([]string{"1", "Alice"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		files := mgr.Files()
		if len(files) != 1 {
			t.Fatalf("Files() len = %d, want 1", len(files))
		}
		if !strings.HasSuffix(files[0], ".tsv") {
			t.Errorf("file path %q does not end with .tsv", files[0])
		}
		data, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if !strings.Contains(string(data), "\t") {
			t.Errorf("TSV file contains no tab: %s", data)
		}
	})

	t.Run("PIPE format produces .txt file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		mgr := New(&LocalWriter{}, "account", time.Now(), format.FormatPIPE, nil, dir)

		if err := mgr.WriteHeader([]string{"id", "name"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := mgr.WriteRow([]string{"1", "Alice"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		files := mgr.Files()
		if len(files) != 1 {
			t.Fatalf("Files() len = %d, want 1", len(files))
		}
		if !strings.HasSuffix(files[0], ".txt") {
			t.Errorf("file path %q does not end with .txt", files[0])
		}
		data, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if !strings.Contains(string(data), "|") {
			t.Errorf("PIPE file contains no pipe: %s", data)
		}
	})
}

func mustParsePartition(t *testing.T, raw string) *partition.Partition {
	t.Helper()
	p, err := partition.Parse(raw)
	if err != nil {
		t.Fatalf("partition.Parse(%q): %v", raw, err)
	}
	return p
}

func TestOutputManagerPartitioned(t *testing.T) {
	t.Parallel()

	t.Run("DATERUN creates single subdirectory", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		startedAt := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
		p := mustParsePartition(t, "DATERUN=yyyy-mm-dd")
		mgr := New(&LocalWriter{}, "account", startedAt, format.FormatCSV, p, dir)

		if err := mgr.WriteHeader([]string{"id"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := mgr.WriteRow([]string{"1"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		files := mgr.Files()
		if len(files) != 1 {
			t.Fatalf("Files() len = %d, want 1", len(files))
		}
		// Path must contain the partition subdir: account/2026-05-18/
		want := filepath.Join(dir, "account", "2026-05-18")
		if !strings.HasPrefix(files[0], want) {
			t.Errorf("file path %q does not start with %q", files[0], want)
		}
	})

	t.Run("DATEMOD two dates create two subdirectories each with header", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		p := mustParsePartition(t, "DATEMOD=yyyy-mm")
		mgr := New(&LocalWriter{}, "account", time.Now(), format.FormatCSV, p, dir)

		if err := mgr.WriteHeader([]string{"id", "name"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		// April row
		if err := mgr.WriteRow([]string{"1", "Alice"}, "2026-04-15T10:00:00Z"); err != nil {
			t.Fatalf("WriteRow April: %v", err)
		}
		// May row
		if err := mgr.WriteRow([]string{"2", "Bob"}, "2026-05-03T10:00:00Z"); err != nil {
			t.Fatalf("WriteRow May: %v", err)
		}
		// Second April row (same handle reused)
		if err := mgr.WriteRow([]string{"3", "Carol"}, "2026-04-20T10:00:00Z"); err != nil {
			t.Fatalf("WriteRow April2: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		files := mgr.Files()
		if len(files) != 2 {
			t.Fatalf("Files() = %v, want 2 files (one per month)", files)
		}

		// Each file must have a header row.
		for _, f := range files {
			of, openErr := os.Open(f)
			if openErr != nil {
				t.Fatalf("open %s: %v", f, openErr)
			}
			t.Cleanup(func() { _ = of.Close() })
			records, parseErr := csv.NewReader(of).ReadAll()
			if parseErr != nil {
				t.Fatalf("parse %s: %v", f, parseErr)
			}
			if len(records) < 1 || records[0][0] != "id" {
				t.Errorf("file %s missing header, records = %v", f, records)
			}
		}

		// Subdirectories must be 2026-04 and 2026-05.
		var months []string
		for _, f := range files {
			parts := strings.Split(filepath.ToSlash(f), "/")
			// path is .../account/2026-05/account-*.csv — find the month segment
			for i, seg := range parts {
				if seg == "account" && i+1 < len(parts) {
					months = append(months, parts[i+1])
					break
				}
			}
		}
		hasApril, hasMay := false, false
		for _, m := range months {
			if m == "2026-04" {
				hasApril = true
			}
			if m == "2026-05" {
				hasMay = true
			}
		}
		if !hasApril {
			t.Errorf("missing 2026-04 partition, months = %v", months)
		}
		if !hasMay {
			t.Errorf("missing 2026-05 partition, months = %v", months)
		}
	})

	t.Run("DATEMOD null modified_at routes to unknown partition", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		p := mustParsePartition(t, "DATEMOD=yyyy-mm")
		mgr := New(&LocalWriter{}, "account", time.Now(), format.FormatCSV, p, dir)

		if err := mgr.WriteHeader([]string{"id"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := mgr.WriteRow([]string{"1"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		files := mgr.Files()
		if len(files) != 1 {
			t.Fatalf("Files() len = %d, want 1", len(files))
		}
		if !strings.Contains(filepath.ToSlash(files[0]), "/unknown/") {
			t.Errorf("null modifiedAt file %q not in unknown/ partition", files[0])
		}
	})

	t.Run("DATERUN files from previous runs are preserved", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// Create a file written by a previous run in a different partition subdir.
		prevDir := filepath.Join(dir, "account", "2026-05-17")
		if err := os.MkdirAll(prevDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		previous := filepath.Join(prevDir, "account-20260517-120000.csv")
		if err := os.WriteFile(previous, []byte("previous run data"), 0o644); err != nil {
			t.Fatalf("write previous: %v", err)
		}

		startedAt := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
		p := mustParsePartition(t, "DATERUN=yyyy-mm-dd")
		mgr := New(&LocalWriter{}, "account", startedAt, format.FormatCSV, p, dir)

		if err := mgr.WriteHeader([]string{"id"}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := mgr.WriteRow([]string{"1"}, ""); err != nil {
			t.Fatalf("WriteRow: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		// File from the previous run must still exist.
		if _, err := os.Stat(previous); err != nil {
			t.Errorf("file from previous run was deleted: %s", previous)
		}
	})
}
