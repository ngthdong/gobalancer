package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPChecker struct {
	client *http.Client
	path   string
}

func NewHTTPChecker(timeout time.Duration, path string) *HTTPChecker {
	return &HTTPChecker{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse 
			},
		},
		path: path,
	}
}

func (h *HTTPChecker) Check(ctx context.Context, addr string) error {
	url := fmt.Sprintf("http://%s%s", addr, h.path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	// Identify the health check traffic in backend logs.
	req.Header.Set("User-Agent", "gobalancer-healthcheck/1.0")

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("check request: %w", err)
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unhealthy status: %d", resp.StatusCode)
	}

	return nil
}
