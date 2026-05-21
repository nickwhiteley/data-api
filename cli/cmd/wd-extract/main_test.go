package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nickwhiteley/data-api/cli/internal/config"
	"github.com/nickwhiteley/data-api/cli/internal/format"
	"github.com/nickwhiteley/data-api/cli/internal/wdapi"
)

// pageResponse returns a minimal single-page data response.
// Each data row has one field per column, all set to "val".
func pageResponse(cols []string, numRows int) string {
	colsJSON := `[`
	for i, c := range cols {
		if i > 0 {
			colsJSON += ","
		}
		colsJSON += fmt.Sprintf("%q", c)
	}
	colsJSON += `]`

	rowJSON := `[`
	for i := range cols {
		if i > 0 {
			rowJSON += ","
		}
		rowJSON += `"val"`
	}
	rowJSON += `]`

	rowsJSON := `[`
	for i := range numRows {
		if i > 0 {
			rowsJSON += ","
		}
		rowsJSON += rowJSON
	}
	rowsJSON += `]`
	return fmt.Sprintf(`{"data_extraction_execution_id":"exec-1","columns":%s,"rows":%s}`, colsJSON, rowsJSON)
}

func TestRunNamedTableNotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		APIKey:    "test-key",
		APIURL:    srv.URL,
		RowCount:  1000,
		OutputDir: t.TempDir(),
		Format:    format.FormatCSV,
		Table:     "nonexistent",
	}

	err := run(cfg)
	if err == nil {
		t.Fatal("expected error for named 404 table, got nil")
	}
	var notFound *wdapi.TableNotFoundError
	if !errors.As(err, &notFound) {
		t.Errorf("expected TableNotFoundError, got %T: %v", err, err)
	}
}

func TestRunNamedTableSuccess(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, pageResponse([]string{"id"}, 1)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		APIKey:    "test-key",
		APIURL:    srv.URL,
		RowCount:  1000,
		OutputDir: t.TempDir(),
		Format:    format.FormatCSV,
		Table:     "account",
	}

	if err := run(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunTablesListSuccess(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, pageResponse([]string{"id"}, 0)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		APIKey:    "test-key",
		APIURL:    srv.URL,
		RowCount:  1000,
		OutputDir: t.TempDir(),
		Format:    format.FormatCSV,
		Tables:    []string{"account", "transaction"},
	}

	if err := run(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDiscoverTablesSkippedWhenNamed(t *testing.T) {
	t.Parallel()

	discoverCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/data/extract" && r.Method == http.MethodGet {
			// DiscoverTables endpoint — should not be called for named selection.
			q := r.URL.Query()
			if q.Get("row_count") == "" {
				discoverCalled = true
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, pageResponse([]string{"id"}, 0)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		APIKey:    "test-key",
		APIURL:    srv.URL,
		RowCount:  1000,
		OutputDir: t.TempDir(),
		Format:    format.FormatCSV,
		Table:     "account",
	}

	if err := run(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if discoverCalled {
		t.Error("DiscoverTables was called despite -t flag being set")
	}
}

func TestRunSmokeTest3Pages500Rows(t *testing.T) {
	t.Parallel()

	const rowsPerPage = 500
	const totalRows = 1500 // 3 full pages + 1 empty terminator

	callNum := atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// DiscoverTables: GET /api/v1/data/extract (no table path segment)
		if r.URL.Path == "/api/v1/data/extract" && r.URL.Query().Get("row_count") == "" {
			if _, err := fmt.Fprint(w, `{"tables":[{"table_name":"account"}]}`); err != nil {
				t.Errorf("write discover: %v", err)
			}
			return
		}

		// PageExtract: GET /api/v1/data/extract/account?row_count=...&page_number=N
		n := callNum.Add(1)
		var rows int
		if n <= 3 {
			rows = rowsPerPage // pages 1-3: 500 rows each
		}
		// page 4: 0 rows → triggers loop break
		body := pageResponse([]string{"id", "name"}, rows)
		if _, err := fmt.Fprint(w, body); err != nil {
			t.Errorf("write page %d: %v", n, err)
		}
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cfg := &config.Config{
		APIKey:    "test-key",
		APIURL:    srv.URL,
		RowCount:  rowsPerPage,
		OutputDir: dir,
		Format:    format.FormatCSV,
	}

	if err := run(cfg); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	// Find the output file.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read output dir: %v", err)
	}
	var csvFiles []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "account-") && strings.HasSuffix(e.Name(), ".csv") {
			csvFiles = append(csvFiles, filepath.Join(dir, e.Name()))
		}
	}
	if len(csvFiles) != 1 {
		t.Fatalf("expected 1 output file, got %d: %v", len(csvFiles), csvFiles)
	}

	f, err := os.Open(csvFiles[0])
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}

	// 1 header row + totalRows data rows.
	wantRecords := 1 + totalRows
	if len(records) != wantRecords {
		t.Errorf("records = %d, want %d (1 header + %d data rows)", len(records), wantRecords, totalRows)
	}

	// Header must be first row.
	if len(records) > 0 && (records[0][0] != "id" || records[0][1] != "name") {
		t.Errorf("header = %v, want [id name]", records[0])
	}

	// Confirm page count: 3 full pages + 1 empty page (terminator).
	if got := callNum.Load(); got != 4 {
		t.Errorf("page requests = %d, want 4 (3 full + 1 empty terminator)", got)
	}
}
