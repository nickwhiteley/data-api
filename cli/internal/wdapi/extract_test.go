package wdapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

// pageResponse constructs a standard page JSON response.
func pageResponse(executionID string, columns []string, rows [][]any) string {
	rowsJSON, _ := json.Marshal(rows)
	colsJSON, _ := json.Marshal(columns)
	return fmt.Sprintf(`{"data_extraction_execution_id":%q,"columns":%s,"rows":%s}`,
		executionID, colsJSON, rowsJSON)
}

// writeJSON writes a JSON string to a ResponseWriter, failing the test on error.
func writeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := fmt.Fprint(w, body); err != nil {
		t.Errorf("writeJSON: %v", err)
	}
}

func TestPageExtract(t *testing.T) {
	t.Parallel()

	t.Run("single page happy path", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, pageResponse("exec-1", []string{"id", "name"}, [][]any{
				{json.Number("1"), "Alice"},
				{json.Number("2"), "Bob"},
				{json.Number("3"), "Carol"},
			}))
		}))
		t.Cleanup(srv.Close)

		client := NewClient(srv.URL, "test-key")
		page, err := PageExtract(context.Background(), client, PageExtractInput{
			Table: "account", PageNum: 1, RowCount: 100,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if page.ExecutionID != "exec-1" {
			t.Errorf("ExecutionID = %q, want %q", page.ExecutionID, "exec-1")
		}
		if len(page.Columns) != 2 || page.Columns[0] != "id" || page.Columns[1] != "name" {
			t.Errorf("Columns = %v, want [id name]", page.Columns)
		}
		if len(page.Rows) != 3 {
			t.Errorf("len(Rows) = %d, want 3", len(page.Rows))
		}
	})

	t.Run("two page pagination passes execution id", func(t *testing.T) {
		t.Parallel()

		var receivedExecID string
		callNum := atomic.Int32{}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := callNum.Add(1)
			if n == 1 {
				writeJSON(t, w, pageResponse("exec-abc", []string{"id"}, [][]any{{json.Number("1")}}))
				return
			}
			receivedExecID = r.URL.Query().Get("data_extraction_execution_id")
			writeJSON(t, w, pageResponse("exec-abc", []string{"id"}, [][]any{{json.Number("2")}}))
		}))
		t.Cleanup(srv.Close)

		client := NewClient(srv.URL, "test-key")

		// Page 1
		page1, err := PageExtract(context.Background(), client, PageExtractInput{
			Table: "account", PageNum: 1, RowCount: 1,
		})
		if err != nil {
			t.Fatalf("page 1 error: %v", err)
		}

		// Page 2 — must include executionID
		_, err = PageExtract(context.Background(), client, PageExtractInput{
			Table: "account", PageNum: 2, ExecutionID: page1.ExecutionID, RowCount: 1,
		})
		if err != nil {
			t.Fatalf("page 2 error: %v", err)
		}
		if receivedExecID != "exec-abc" {
			t.Errorf("page 2 execution ID = %q, want %q", receivedExecID, "exec-abc")
		}
	})

	t.Run("429 retried twice then success", func(t *testing.T) {
		t.Parallel()

		calls := atomic.Int32{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := calls.Add(1)
			if n <= 2 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			writeJSON(t, w, pageResponse("exec-2", []string{"x"}, [][]any{{json.Number("1")}}))
		}))
		t.Cleanup(srv.Close)

		// Use a client with very short timeouts for test speed — we need to override
		// retryDo delays. Since we can't mock time easily, use a real server and accept
		// the real delays only for a small number of retries. Actually let's keep the
		// delays but use a short server. The test will take ~3s (1+2s) which is acceptable.
		client := NewClient(srv.URL, "test-key")
		page, err := PageExtract(context.Background(), client, PageExtractInput{
			Table: "t", PageNum: 1, RowCount: 10,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls.Load() != 3 {
			t.Errorf("server calls = %d, want 3", calls.Load())
		}
		if page.ExecutionID != "exec-2" {
			t.Errorf("ExecutionID = %q, want exec-2", page.ExecutionID)
		}
	})

	t.Run("5xx retried then success", func(t *testing.T) {
		t.Parallel()

		calls := atomic.Int32{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := calls.Add(1)
			if n == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			writeJSON(t, w, pageResponse("exec-3", []string{"a"}, [][]any{{json.Number("7")}}))
		}))
		t.Cleanup(srv.Close)

		client := NewClient(srv.URL, "test-key")
		_, err := PageExtract(context.Background(), client, PageExtractInput{
			Table: "t", PageNum: 1, RowCount: 10,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls.Load() != 2 {
			t.Errorf("server calls = %d, want 2", calls.Load())
		}
	})

	t.Run("all retries exhausted returns error", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(srv.Close)

		client := NewClient(srv.URL, "test-key")
		_, err := PageExtract(context.Background(), client, PageExtractInput{
			Table: "t", PageNum: 1, RowCount: 10,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("403 returns plain error not TableNotFoundError", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(srv.Close)

		client := NewClient(srv.URL, "test-key")
		_, err := PageExtract(context.Background(), client, PageExtractInput{
			Table: "t", PageNum: 1, RowCount: 10,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var notFound *TableNotFoundError
		if errors.As(err, &notFound) {
			t.Error("expected plain error for 403, got TableNotFoundError")
		}
	})

	t.Run("404 returns TableNotFoundError", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)

		client := NewClient(srv.URL, "test-key")
		_, err := PageExtract(context.Background(), client, PageExtractInput{
			Table: "missing_table", PageNum: 1, RowCount: 10,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var notFound *TableNotFoundError
		if !errors.As(err, &notFound) {
			t.Errorf("expected TableNotFoundError, got %T: %v", err, err)
		}
		if notFound.Table != "missing_table" {
			t.Errorf("TableNotFoundError.Table = %q, want %q", notFound.Table, "missing_table")
		}
	})

	t.Run("json.Number large integer preserved exactly", func(t *testing.T) {
		t.Parallel()

		const bigInt = "9999999999999999"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// Send the large integer as a raw JSON number (not a string).
			body := fmt.Sprintf(
				`{"data_extraction_execution_id":"exec-big","columns":["n"],"rows":[[%s]]}`,
				bigInt,
			)
			writeJSON(t, w, body)
		}))
		t.Cleanup(srv.Close)

		client := NewClient(srv.URL, "test-key")
		page, err := PageExtract(context.Background(), client, PageExtractInput{
			Table: "t", PageNum: 1, RowCount: 10,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(page.Rows) != 1 || len(page.Rows[0]) != 1 {
			t.Fatal("unexpected rows shape")
		}
		num, ok := page.Rows[0][0].(json.Number)
		if !ok {
			t.Fatalf("cell type = %T, want json.Number", page.Rows[0][0])
		}
		if num.String() != bigInt {
			t.Errorf("large integer = %q, want %q (precision lost)", num.String(), bigInt)
		}
	})

	t.Run("first page URL has no since or from_date param", func(t *testing.T) {
		t.Parallel()

		var rawQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawQuery = r.URL.RawQuery
			writeJSON(t, w, pageResponse("exec-x", []string{"id"}, [][]any{}))
		}))
		t.Cleanup(srv.Close)

		client := NewClient(srv.URL, "test-key")
		_, err := PageExtract(context.Background(), client, PageExtractInput{
			Table: "t", PageNum: 1, RowCount: 10,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		params, parseErr := url.ParseQuery(rawQuery)
		if parseErr != nil {
			t.Fatalf("parse query %q: %v", rawQuery, parseErr)
		}
		if params.Get("since") != "" {
			t.Errorf("unexpected 'since' param in first-page URL: %s", rawQuery)
		}
		if params.Get("from_date") != "" {
			t.Errorf("unexpected 'from_date' param in first-page URL: %s", rawQuery)
		}
	})
}
