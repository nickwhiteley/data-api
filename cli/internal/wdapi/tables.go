package wdapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// tablesResponse is the JSON shape returned by the Data API tables discovery endpoint.
type tablesResponse struct {
	Tables []struct {
		TableName string `json:"table_name"`
	} `json:"tables"`
}

// DiscoverTables returns the ordered list of table names available for extraction.
func DiscoverTables(ctx context.Context, c *Client) (tables []string, err error) {
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/data/extract", nil)
	if reqErr != nil {
		return nil, fmt.Errorf("build discover request: %w", reqErr)
	}

	resp, doErr := c.retryDo(ctx, req)
	if doErr != nil {
		return nil, doErr
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close discover response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discover tables: unexpected status %d", resp.StatusCode)
	}

	var raw tablesResponse
	if decErr := json.NewDecoder(resp.Body).Decode(&raw); decErr != nil {
		return nil, fmt.Errorf("decode discover response: %w", decErr)
	}

	names := make([]string, 0, len(raw.Tables))
	for _, t := range raw.Tables {
		names = append(names, t.TableName)
	}
	return names, nil
}
