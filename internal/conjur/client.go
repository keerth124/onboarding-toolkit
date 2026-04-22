// Package conjur provides a client for Secrets Manager SaaS and self-hosted Conjur APIs.
package conjur

import (
	"bytes"
	"context"
	"crypto/tls"
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

// Client is an authenticated HTTP client for Conjur APIs.
type Client struct {
	baseURL        string
	apiBaseURL     string
	stripAPIPrefix bool
	account        string
	token          string // base64-encoded Conjur auth token
	verbose        bool
	http           *http.Client
}

type ClientConfig struct {
	Tenant    string
	ConjurURL string
	Account   string
	Username  string
	APIKey    string
	Verbose   bool
	InsecureSkipTLSVerify bool
}

// NewClient authenticates to a Secrets Manager SaaS tenant and returns a ready client.
// The API key is read from the apiKey parameter (sourced from CONJUR_API_KEY env var by callers).
func NewClient(tenant, username, apiKey string, verbose bool) (*Client, error) {
	return NewClientFromConfig(ClientConfig{
		Tenant:   tenant,
		Account:  "conjur",
		Username: username,
		APIKey:   apiKey,
		Verbose:  verbose,
	})
}

func NewClientFromConfig(cfg ClientConfig) (*Client, error) {
	if cfg.Account == "" {
		cfg.Account = "conjur"
	}
	baseURL, apiBaseURL, stripAPIPrefix, err := clientBaseURLs(cfg)
	if err != nil {
		return nil, err
	}

	httpClient := newHTTPClient(cfg.InsecureSkipTLSVerify)

	// Authenticate: POST <api-base>/authn/{account}/{username}/authenticate
	token, err := authenticate(httpClient, apiBaseURL, cfg.Account, cfg.Username, cfg.APIKey, cfg.Verbose)
	if err != nil {
		return nil, fmt.Errorf("authenticating to %s: %w", apiBaseURL, err)
	}

	return &Client{
		baseURL:        baseURL,
		apiBaseURL:     apiBaseURL,
		stripAPIPrefix: stripAPIPrefix,
		account:        cfg.Account,
		token:          token,
		verbose:        cfg.Verbose,
		http:           httpClient,
	}, nil
}

func newHTTPClient(insecureSkipTLSVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- explicit local-testing flag.
	}
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}
}

// authenticate performs the Conjur authn-local flow and returns the base64-encoded token.
func authenticate(client *http.Client, apiBaseURL, account, username, apiKey string, verbose bool) (string, error) {
	authURL := fmt.Sprintf("%s/authn/%s/%s/authenticate",
		apiBaseURL, url.PathEscape(account), url.PathEscape(username))

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
	fullURL := c.apiURL(path)

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

func (c *Client) apiURL(path string) string {
	return c.apiBaseURL + normalizeAPIPath(path, c.stripAPIPrefix, c.account)
}

func tenantBaseURL(tenant string) string {
	return fmt.Sprintf("https://%s.secretsmgr.cyberark.cloud", strings.TrimSuffix(tenant, "/"))
}

func tenantAPIBaseURL(tenant string) string {
	return tenantBaseURL(tenant) + "/api"
}

func clientBaseURLs(cfg ClientConfig) (string, string, bool, error) {
	if strings.TrimSpace(cfg.ConjurURL) != "" {
		baseURL, apiBaseURL, err := normalizeConjurURL(cfg.ConjurURL)
		return baseURL, apiBaseURL, false, err
	}
	if strings.TrimSpace(cfg.Tenant) == "" {
		return "", "", false, fmt.Errorf("tenant or conjur URL is required")
	}
	baseURL := tenantBaseURL(cfg.Tenant)
	return baseURL, baseURL + "/api", true, nil
}

func normalizeConjurURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("conjur URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("parsing conjur URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("conjur URL must include scheme and host")
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), parsed.String(), nil
}

func normalizeAPIPath(path string, stripAPIPrefix bool, account string) string {
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !stripAPIPrefix {
		return normalizeSelfHostedAPIPath(path, account)
	}
	if path == "/api" {
		return ""
	}
	if strings.HasPrefix(path, "/api/") {
		return strings.TrimPrefix(path, "/api")
	}
	return path
}

func normalizeSelfHostedAPIPath(path string, account string) string {
	if account == "" {
		account = "conjur"
	}
	accountPath := url.PathEscape(account)

	switch {
	case path == "/api/authenticators" || path == "/api/authenticators/":
		return "/authenticators/" + accountPath
	case strings.HasPrefix(path, "/api/authenticators/"):
		return "/authenticators/" + accountPath + strings.TrimPrefix(path, "/api/authenticators")
	case path == "/authenticators" || path == "/authenticators/":
		return "/authenticators/" + accountPath
	case path == "/authenticators/{account}":
		return "/authenticators/" + accountPath
	case strings.HasPrefix(path, "/authenticators/{account}/"):
		return "/authenticators/" + accountPath + strings.TrimPrefix(path, "/authenticators/{account}")
	default:
		return path
	}
}
