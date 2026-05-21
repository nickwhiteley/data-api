package wdapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// TableNotFoundError is returned when the API responds with 404 for a table.
type TableNotFoundError struct {
	Table string
}

// Error implements the error interface.
func (e *TableNotFoundError) Error() string {
	return fmt.Sprintf("table not found: %s", e.Table)
}

// DJSONPage holds a single page of columnar data returned by the Data API.
type DJSONPage struct {
	ExecutionID string
	Columns     []string
	Rows        [][]any
}

// PageExtractInput holds the parameters for a single page extraction request.
type PageExtractInput struct {
	Table       string
	PageNum     int
	ExecutionID string
	RowCount    int
}

// PageExtract fetches one page of data for the given table.
// It returns a *TableNotFoundError when the server responds with 404.
func PageExtract(ctx context.Context, c *Client, in PageExtractInput) (page *DJSONPage, err error) {
	u, parseErr := url.Parse(c.baseURL + "/api/v1/data/extract/" + in.Table)
	if parseErr != nil {
		return nil, fmt.Errorf("build extract URL: %w", parseErr)
	}

	q := u.Query()
	q.Set("row_count", strconv.Itoa(in.RowCount))
	q.Set("page_number", strconv.Itoa(in.PageNum))
	if in.ExecutionID != "" {
		q.Set("data_extraction_execution_id", in.ExecutionID)
	}
	u.RawQuery = q.Encode()

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if reqErr != nil {
		return nil, fmt.Errorf("build extract request: %w", reqErr)
	}

	resp, doErr := c.retryDo(ctx, req)
	if doErr != nil {
		return nil, doErr
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close extract response body: %w", closeErr)
		}
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &TableNotFoundError{Table: in.Table}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("extract %s: unexpected status %d", in.Table, resp.StatusCode)
	}

	var raw struct {
		ExecutionID string   `json:"data_extraction_execution_id"`
		Columns     []string `json:"columns"`
		Rows        [][]any  `json:"rows"`
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if decErr := dec.Decode(&raw); decErr != nil {
		return nil, fmt.Errorf("decode extract response: %w", decErr)
	}

	return &DJSONPage{
		ExecutionID: raw.ExecutionID,
		Columns:     raw.Columns,
		Rows:        raw.Rows,
	}, nil
}
