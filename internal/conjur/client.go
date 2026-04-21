// Package conjur provides a client for the Secrets Manager SaaS (Conjur Cloud) v2 REST API.
package conjur

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	apiVersion = "application/x.secretsmgr.v2beta+json"
	maxRetries = 3
)

// Client is an authenticated HTTP client for Conjur Cloud.
type Client struct {
	baseURL  string
	token    string // base64-encoded Conjur auth token
	verbose  bool
	http     *http.Client
}

// NewClient authenticates to the Conjur Cloud tenant and returns a ready client.
// The API key is read from the apiKey parameter (sourced from CONJUR_API_KEY env var by callers).
func NewClient(tenant, username, apiKey string, verbose bool) (*Client, error) {
	baseURL := fmt.Sprintf("https://%s.secretsmgr.cyberark.cloud", tenant)

	httpClient := &http.Client{Timeout: 60 * time.Second}

	// Authenticate: POST /authn/conjur/{username}/authenticate
	token, err := authenticate(httpClient, baseURL, username, apiKey, verbose)
	if err != nil {
		return nil, fmt.Errorf("authenticating to %s: %w", baseURL, err)
	}

	return &Client{
		baseURL: baseURL,
		token:   token,
		verbose: verbose,
		http:    httpClient,
	}, nil
}

// authenticate performs the Conjur authn-local flow and returns the base64-encoded token.
func authenticate(client *http.Client, baseURL, username, apiKey string, verbose bool) (string, error) {
	authURL := fmt.Sprintf("%s/authn/conjur/%s/authenticate",
		baseURL, url.PathEscape(username))

	body := strings.NewReader(apiKey)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, authURL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Accept", "text/plain")

	if verbose {
		fmt.Printf("  [auth] POST %s\n", authURL)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST authenticate: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading auth response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("authentication failed (HTTP %d): %s", resp.StatusCode, string(raw))
	}

	// Conjur expects the token base64-encoded in the Authorization header.
	encoded := base64.StdEncoding.EncodeToString(raw)
	return encoded, nil
}

// Post sends an authenticated POST to the given path with the given body.
// Returns (status code, response body, error).
// Retries on 5xx up to maxRetries times with exponential backoff.
func (c *Client) Post(ctx context.Context, path string, contentType string, body []byte) (int, []byte, error) {
	return c.doWithRetry(ctx, http.MethodPost, path, contentType, body)
}

// Delete sends an authenticated DELETE to the given path.
func (c *Client) Delete(ctx context.Context, path string) (int, []byte, error) {
	return c.doWithRetry(ctx, http.MethodDelete, path, "", nil)
}

// Get sends an authenticated GET to the given path.
func (c *Client) Get(ctx context.Context, path string) (int, []byte, error) {
	return c.doWithRetry(ctx, http.MethodGet, path, "", nil)
}

func (c *Client) doWithRetry(ctx context.Context, method, path, contentType string, body []byte) (int, []byte, error) {
	fullURL := c.baseURL + path

	var lastErr error
	backoff := 1 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, nil, ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
			}
		}

		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
		if err != nil {
			return 0, nil, fmt.Errorf("building request: %w", err)
		}

		req.Header.Set("Authorization", fmt.Sprintf("Token token=%q", c.token))
		req.Header.Set("Accept", apiVersion)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		if c.verbose {
			fmt.Printf("  [api] %s %s\n", method, fullURL)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s %s: %w", method, fullURL, err)
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("reading response body: %w", readErr)
			continue
		}

		if c.verbose {
			fmt.Printf("  [api] %d %s\n", resp.StatusCode, string(respBody))
		}

		// Retry on transient server errors only.
		if resp.StatusCode >= 500 && attempt < maxRetries {
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody))
			continue
		}

		return resp.StatusCode, respBody, nil
	}

	return 0, nil, lastErr
}
