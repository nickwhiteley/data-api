package wdapi

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Client is an authenticated HTTP client for the WoodenDollars Data API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient returns a Client configured with the given base URL and API key.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// do sets the Authorization header and executes the request.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", req.URL.Path, err)
	}
	return resp, nil
}

// retryDo executes a request with up to 3 retries on 429 and 5xx responses.
// Backoff delays between retries: 1s, 2s, 4s.
func (c *Client) retryDo(ctx context.Context, req *http.Request) (*http.Response, error) {
	retryDelays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error
	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelays[attempt-1]):
			}
		}
		cloned := req.Clone(ctx)
		resp, err := c.do(cloned)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if err := resp.Body.Close(); err != nil {
				return nil, fmt.Errorf("close retry response body: %w", err)
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("all retries exhausted: %w", lastErr)
}
