package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// fetchBody sends a GET request to baseURL with apiKey as the "key" query
// parameter and returns the successful response body. The caller is
// responsible for closing the returned body.
func fetchBody(ctx context.Context, client *http.Client, baseURL, apiKey string) (io.ReadCloser, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		// Fires on malformed percent-escapes or control characters; callers
		// pass well-formed URLs.
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}
	q := u.Query()
	q.Set("key", apiKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		// Fires on invalid method, invalid URL, or nil ctx; none apply to
		// current callers.
		return nil, fmt.Errorf("creating request to %s: %w", u.Host, err)
	}

	// BrsApi's firewall (their documented "Gen6" WAF) rejects requests using
	// Go's default "Go-http-client" User-Agent; spoofing a common browser UA is
	// a pragmatic workaround for that, not a claim that this program is a
	// browser. Confirmed against BrsApi's own documentation.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := client.Do(req)
	if err != nil {
		// Do not wrap err: *url.Error may contain the request URL, which includes
		// the API key.
		return nil, fmt.Errorf("connecting to %s failed", u.Host)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status from %s: %s", u.Host, resp.Status)
	}
	return resp.Body, nil
}
