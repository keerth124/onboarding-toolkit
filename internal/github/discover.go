// Package github handles GitHub API discovery for the conjur-onboard CLI.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	githubAPIBase = "https://api.github.com"
	pageSize      = 100
)

// DiscoverConfig holds inputs for the discovery operation.
type DiscoverConfig struct {
	Org     string
	Token   string
	Verbose bool
}

// RepoInfo holds per-repository metadata relevant to Conjur onboarding.
type RepoInfo struct {
	Name          string   `json:"name"`
	FullName      string   `json:"full_name"`
	DefaultBranch string   `json:"default_branch"`
	Visibility    string   `json:"visibility"`
	Environments  []string `json:"environments"`
	Archived      bool     `json:"archived"`
}

// DiscoveryResult is the normalized output written to discovery.json.
type DiscoveryResult struct {
	Platform           string     `json:"platform"`
	Org                string     `json:"org"`
	OIDCIssuer         string     `json:"oidc_issuer"`
	JWKSUri            string     `json:"jwks_uri"`
	Repos              []RepoInfo `json:"repos"`
	SubClaimCustomized bool       `json:"sub_claim_customized"`
	DiscoveredAt       string     `json:"discovered_at"`
}

// Discover enumerates repos and OIDC configuration for a GitHub org.
func Discover(ctx context.Context, cfg DiscoverConfig) (*DiscoveryResult, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	repos, err := listRepos(ctx, client, cfg)
	if err != nil {
		return nil, fmt.Errorf("listing repos: %w", err)
	}

	// Fetch environments for each repo concurrently (bounded).
	enriched := make([]RepoInfo, 0, len(repos))
	for _, r := range repos {
		if r.Archived {
			continue
		}
		envs, err := listEnvironments(ctx, client, cfg, r.FullName)
		if err != nil {
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "  warn: could not fetch environments for %s: %v\n", r.FullName, err)
			}
		}
		r.Environments = envs
		enriched = append(enriched, r)

		if cfg.Verbose {
			fmt.Printf("  discovered repo: %s  (environments: %v)\n", r.FullName, envs)
		}
	}

	return &DiscoveryResult{
		Platform:           "github",
		Org:                cfg.Org,
		OIDCIssuer:         "https://token.actions.githubusercontent.com",
		JWKSUri:            "https://token.actions.githubusercontent.com/.well-known/jwks",
		Repos:              enriched,
		SubClaimCustomized: false, // future: detect via org settings API
		DiscoveredAt:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// LoadDiscovery reads discovery.json from wd and returns the parsed result.
func LoadDiscovery(wd string) (*DiscoveryResult, error) {
	path := filepath.Join(wd, "discovery.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var result DiscoveryResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &result, nil
}

// listRepos fetches all non-archived repos for the org (paginated).
func listRepos(ctx context.Context, client *http.Client, cfg DiscoverConfig) ([]RepoInfo, error) {
	var all []RepoInfo
	page := 1
	for {
		url := fmt.Sprintf("%s/orgs/%s/repos?type=all&per_page=%d&page=%d", githubAPIBase, cfg.Org, pageSize, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("GET %s: %w", url, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("GitHub token invalid or missing required scopes (repo, read:org). Run: gh auth refresh -s repo,read:org")
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub API returned %d for org repos", resp.StatusCode)
		}

		var page_repos []struct {
			Name          string `json:"name"`
			FullName      string `json:"full_name"`
			DefaultBranch string `json:"default_branch"`
			Visibility    string `json:"visibility"`
			Archived      bool   `json:"archived"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&page_repos); err != nil {
			return nil, fmt.Errorf("decoding repos response: %w", err)
		}
		if len(page_repos) == 0 {
			break
		}
		for _, r := range page_repos {
			all = append(all, RepoInfo{
				Name:          r.Name,
				FullName:      r.FullName,
				DefaultBranch: r.DefaultBranch,
				Visibility:    r.Visibility,
				Archived:      r.Archived,
			})
		}
		if len(page_repos) < pageSize {
			break
		}
		page++
	}
	return all, nil
}

// listEnvironments returns environment names for a given repo.
func listEnvironments(ctx context.Context, client *http.Client, cfg DiscoverConfig, fullName string) ([]string, error) {
	url := fmt.Sprintf("%s/repos/%s/environments?per_page=100", githubAPIBase, fullName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Repo has no environments — normal for many repos.
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("environments API returned %d", resp.StatusCode)
	}

	var body struct {
		Environments []struct {
			Name string `json:"name"`
		} `json:"environments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(body.Environments))
	for _, e := range body.Environments {
		names = append(names, e.Name)
	}
	return names, nil
}
