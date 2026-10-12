// Package httpjson contains the only outbound HTTP transport used by the AI clients.
// It deliberately depends on the Go standard library, not an AI SDK.
package httpjson

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const MaxResponse = 512 << 10

func Client() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// PostJSON retries temporary 429/5xx failures at most twice. POST is safe to retry
// here because inference requests are read-only; callers must not use this for writes.
func PostJSON(ctx context.Context, c *http.Client, endpoint, bearer string, body []byte, limit int64) ([]byte, error) {
	if c == nil {
		c = Client()
	}
	if limit <= 0 || limit > MaxResponse {
		limit = MaxResponse
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := c.Do(req)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, limit+1))
			resp.Body.Close()
			if readErr != nil {
				return nil, fmt.Errorf("provider response read failed: %w", readErr)
			}
			if int64(len(data)) > limit {
				return nil, errors.New("provider response exceeds configured limit")
			}
			if resp.StatusCode == http.StatusOK {
				return data, nil
			}
			retryable := resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504
			if !retryable || attempt == 2 {
				return nil, fmt.Errorf("provider HTTP %d", resp.StatusCode)
			}
		} else if attempt == 2 || ctx.Err() != nil {
			return nil, fmt.Errorf("provider connection failed: %w", err)
		}
		wait := time.Duration(250*(1<<attempt)) * time.Millisecond
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, errors.New("request attempts exceeded")
}
