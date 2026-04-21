// Package github handles GitHub API discovery for the conjur-onboard CLI.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	githubAPIBase = "https://api.github.com"
	pageSize      = 100
)

// DiscoverConfig holds inputs for the discovery operation.
type DiscoverConfig struct {
	Org       string
	Token     string
	RepoNames []string
	Verbose   bool
}

// OrgInfo holds organization metadata relevant to discovery and review.
type OrgInfo struct {
	ID            int64    `json:"id"`
	Login         string   `json:"login"`
	Name          string   `json:"name,omitempty"`
	AccountType   string   `json:"account_type,omitempty"`
	Authenticated bool     `json:"authenticated,omitempty"`
	NodeID        string   `json:"node_id,omitempty"`
	PublicRepos   int      `json:"public_repos,omitempty"`
	PlanName      string   `json:"plan_name,omitempty"`
	PlanSpace     int      `json:"plan_space,omitempty"`
	PrivateRepos  int      `json:"private_repos,omitempty"`
	Enterprise    string   `json:"enterprise,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
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

// OIDCSubCustomization records GitHub Actions OIDC subject customization.
type OIDCSubCustomization struct {
	Detected         bool     `json:"detected"`
	IncludeClaimKeys []string `json:"include_claim_keys,omitempty"`
	Warning          string   `json:"warning,omitempty"`
}

// DiscoveryResult is the normalized output written to discovery.json.
type DiscoveryResult struct {
	Platform             string               `json:"platform"`
	Org                  string               `json:"org"`
	OrgInfo              OrgInfo              `json:"org_info"`
	OIDCIssuer           string               `json:"oidc_issuer"`
	JWKSUri              string               `json:"jwks_uri"`
	Repos                []RepoInfo           `json:"repos"`
	SubClaimCustomized   bool                 `json:"sub_claim_customized"`
	OIDCSubCustomization OIDCSubCustomization `json:"oidc_sub_customization"`
	Warnings             []string             `json:"warnings,omitempty"`
	DiscoveredAt         string               `json:"discovered_at"`
}

// Discover enumerates repos and OIDC configuration for a GitHub org.
func Discover(ctx context.Context, cfg DiscoverConfig) (*DiscoveryResult, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	orgInfo, err := getOrgInfo(ctx, client, cfg)
	if err != nil {
		return nil, fmt.Errorf("getting org metadata: %w", err)
	}

	oidcCustomization := OIDCSubCustomization{}
	if orgInfo.AccountType == "Organization" {
		var err error
		oidcCustomization, err = getOIDCSubCustomization(ctx, client, cfg)
		if err != nil {
			return nil, fmt.Errorf("getting OIDC subject customization: %w", err)
		}
	} else {
		oidcCustomization.Warning = fmt.Sprintf("GitHub owner %q is a %s account; org-level OIDC subject customization was not checked", cfg.Org, strings.ToLower(orgInfo.AccountType))
	}

	repos, err := discoverRepos(ctx, client, cfg, orgInfo)
	if err != nil {
		return nil, fmt.Errorf("listing repos: %w", err)
	}

	enriched := make([]RepoInfo, 0, len(repos))
	var warnings []string
	for _, repo := range repos {
		if repo.Archived {
			continue
		}

		envs, err := listEnvironments(ctx, client, cfg, repo.FullName)
		if err != nil {
			warning := fmt.Sprintf("could not fetch environments for %s: %v", repo.FullName, err)
			warnings = append(warnings, warning)
			verbosef(cfg, "  warn: %s\n", warning)
		}
		repo.Environments = envs
		enriched = append(enriched, repo)

		verbosef(cfg, "  discovered repo: %s (environments: %v)\n", repo.FullName, envs)
	}

	if oidcCustomization.Warning != "" {
		warnings = append(warnings, oidcCustomization.Warning)
	}

	return &DiscoveryResult{
		Platform:             "github",
		Org:                  cfg.Org,
		OrgInfo:              orgInfo,
		OIDCIssuer:           "https://token.actions.githubusercontent.com",
		JWKSUri:              "https://token.actions.githubusercontent.com/.well-known/jwks",
		Repos:                enriched,
		SubClaimCustomized:   oidcCustomization.Detected,
		OIDCSubCustomization: oidcCustomization,
		Warnings:             warnings,
		DiscoveredAt:         time.Now().UTC().Format(time.RFC3339),
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

func discoverRepos(ctx context.Context, client *http.Client, cfg DiscoverConfig, orgInfo OrgInfo) ([]RepoInfo, error) {
	if len(cfg.RepoNames) > 0 {
		return getSelectedRepos(ctx, client, cfg)
	}
	return listRepos(ctx, client, cfg, orgInfo)
}

func getOrgInfo(ctx context.Context, client *http.Client, cfg DiscoverConfig) (OrgInfo, error) {
	url := fmt.Sprintf("%s/orgs/%s", githubAPIBase, cfg.Org)

	var body struct {
		ID           int64  `json:"id"`
		Login        string `json:"login"`
		Name         string `json:"name"`
		NodeID       string `json:"node_id"`
		PrivateRepos int    `json:"total_private_repos"`
		Plan         struct {
			Name  string `json:"name"`
			Space int    `json:"space"`
		} `json:"plan"`
		Enterprise struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"enterprise"`
		Type string `json:"type"`
	}

	resp, err := getJSON(ctx, client, cfg, url, &body)
	if err != nil {
		return OrgInfo{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return OrgInfo{}, authError(resp, "read:org")
	}
	if resp.StatusCode == http.StatusNotFound {
		return getUserInfo(ctx, client, cfg)
	}
	if resp.StatusCode != http.StatusOK {
		return OrgInfo{}, fmt.Errorf("GitHub API returned %d for org metadata", resp.StatusCode)
	}

	enterprise := body.Enterprise.Slug
	if enterprise == "" {
		enterprise = body.Enterprise.Name
	}

	return OrgInfo{
		ID:           body.ID,
		Login:        body.Login,
		Name:         body.Name,
		AccountType:  "Organization",
		NodeID:       body.NodeID,
		PlanName:     body.Plan.Name,
		PlanSpace:    body.Plan.Space,
		PrivateRepos: body.PrivateRepos,
		Enterprise:   enterprise,
	}, nil
}

func getUserInfo(ctx context.Context, client *http.Client, cfg DiscoverConfig) (OrgInfo, error) {
	url := fmt.Sprintf("%s/users/%s", githubAPIBase, cfg.Org)

	var body struct {
		ID          int64  `json:"id"`
		Login       string `json:"login"`
		Name        string `json:"name"`
		NodeID      string `json:"node_id"`
		PublicRepos int    `json:"public_repos"`
		Type        string `json:"type"`
	}

	resp, err := getJSON(ctx, client, cfg, url, &body)
	if err != nil {
		return OrgInfo{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return OrgInfo{}, authError(resp, "repo")
	}
	if resp.StatusCode == http.StatusNotFound {
		return OrgInfo{}, fmt.Errorf("GitHub owner %q was not found or is not visible to the token", cfg.Org)
	}
	if resp.StatusCode != http.StatusOK {
		return OrgInfo{}, fmt.Errorf("GitHub API returned %d for owner metadata", resp.StatusCode)
	}

	accountType := body.Type
	if accountType == "" {
		accountType = "User"
	}
	authenticatedLogin, err := getAuthenticatedUserLogin(ctx, client, cfg)
	if err != nil {
		return OrgInfo{}, err
	}

	return OrgInfo{
		ID:            body.ID,
		Login:         body.Login,
		Name:          body.Name,
		AccountType:   accountType,
		Authenticated: strings.EqualFold(body.Login, authenticatedLogin),
		NodeID:        body.NodeID,
		PublicRepos:   body.PublicRepos,
	}, nil
}

func getAuthenticatedUserLogin(ctx context.Context, client *http.Client, cfg DiscoverConfig) (string, error) {
	if cfg.Token == "" {
		return "", nil
	}

	url := githubAPIBase + "/user"
	var body struct {
		Login string `json:"login"`
	}

	resp, err := getJSON(ctx, client, cfg, url, &body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", authError(resp, "repo")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d for authenticated user metadata", resp.StatusCode)
	}
	return body.Login, nil
}

func getOIDCSubCustomization(ctx context.Context, client *http.Client, cfg DiscoverConfig) (OIDCSubCustomization, error) {
	url := fmt.Sprintf("%s/orgs/%s/actions/oidc/customization/sub", githubAPIBase, cfg.Org)

	var body struct {
		IncludeClaimKeys []string `json:"include_claim_keys"`
	}

	resp, err := getJSON(ctx, client, cfg, url, &body)
	if err != nil {
		return OIDCSubCustomization{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return OIDCSubCustomization{}, authError(resp, "read:org")
	}
	if resp.StatusCode == http.StatusNotFound {
		return OIDCSubCustomization{
			Detected: false,
			Warning:  "OIDC subject customization endpoint returned 404; org-level customization could not be detected",
		}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return OIDCSubCustomization{}, fmt.Errorf("GitHub API returned %d for org OIDC subject customization", resp.StatusCode)
	}

	sort.Strings(body.IncludeClaimKeys)
	return OIDCSubCustomization{
		Detected:         len(body.IncludeClaimKeys) > 0,
		IncludeClaimKeys: body.IncludeClaimKeys,
	}, nil
}

func getSelectedRepos(ctx context.Context, client *http.Client, cfg DiscoverConfig) ([]RepoInfo, error) {
	repos := make([]RepoInfo, 0, len(cfg.RepoNames))
	for _, repoName := range cfg.RepoNames {
		fullName := normalizeRepoName(cfg.Org, repoName)
		url := fmt.Sprintf("%s/repos/%s", githubAPIBase, fullName)

		var body repoResponse
		resp, err := getJSON(ctx, client, cfg, url, &body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, authError(resp, "repo")
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("repository %q was not found or is not visible to the token", fullName)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub API returned %d for repository %s", resp.StatusCode, fullName)
		}

		repos = append(repos, body.toRepoInfo())
	}
	return repos, nil
}

// listRepos fetches all repos visible to the token for the org.
func listRepos(ctx context.Context, client *http.Client, cfg DiscoverConfig, orgInfo OrgInfo) ([]RepoInfo, error) {
	var all []RepoInfo
	page := 1
	for {
		url := fmt.Sprintf("%s/orgs/%s/repos?type=all&per_page=%d&page=%d", githubAPIBase, cfg.Org, pageSize, page)
		if orgInfo.AccountType != "Organization" && orgInfo.Authenticated {
			url = fmt.Sprintf("%s/user/repos?visibility=all&affiliation=owner&per_page=%d&page=%d", githubAPIBase, pageSize, page)
		} else if orgInfo.AccountType != "Organization" {
			url = fmt.Sprintf("%s/users/%s/repos?type=owner&per_page=%d&page=%d", githubAPIBase, cfg.Org, pageSize, page)
		}

		var pageRepos []repoResponse
		resp, err := getJSON(ctx, client, cfg, url, &pageRepos)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, authError(resp, "repo")
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("GitHub owner %q was not found or repositories are not visible to the token", cfg.Org)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub API returned %d for org repos", resp.StatusCode)
		}

		if len(pageRepos) == 0 {
			break
		}
		for _, repo := range pageRepos {
			all = append(all, repo.toRepoInfo())
		}
		if len(pageRepos) < pageSize {
			break
		}
		page++
	}
	return all, nil
}

// listEnvironments returns environment names for a given repo.
func listEnvironments(ctx context.Context, client *http.Client, cfg DiscoverConfig, fullName string) ([]string, error) {
	var names []string
	page := 1
	for {
		url := fmt.Sprintf("%s/repos/%s/environments?per_page=%d&page=%d", githubAPIBase, fullName, pageSize, page)

		var body struct {
			Environments []struct {
				Name string `json:"name"`
			} `json:"environments"`
		}

		resp, err := getJSON(ctx, client, cfg, url, &body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, authError(resp, "repo")
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("environments API returned %d", resp.StatusCode)
		}

		for _, env := range body.Environments {
			names = append(names, env.Name)
		}
		if len(body.Environments) < pageSize {
			break
		}
		page++
	}

	sort.Strings(names)
	return names, nil
}

type repoResponse struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Visibility    string `json:"visibility"`
	Archived      bool   `json:"archived"`
}

func (r repoResponse) toRepoInfo() RepoInfo {
	return RepoInfo{
		Name:          r.Name,
		FullName:      r.FullName,
		DefaultBranch: r.DefaultBranch,
		Visibility:    r.Visibility,
		Archived:      r.Archived,
	}
}

func getJSON(ctx context.Context, client *http.Client, cfg DiscoverConfig, url string, dest any) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	setGitHubHeaders(req, cfg.Token)

	verbosef(cfg, "  [github] GET %s\n", url)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body)
		return resp, nil
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return nil, fmt.Errorf("decoding %s response: %w", url, err)
	}
	return resp, nil
}

func setGitHubHeaders(req *http.Request, token string) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

func authError(resp *http.Response, requiredScopes ...string) error {
	current := parseScopes(resp.Header.Get("X-OAuth-Scopes"))
	missing := missingScopes(current, requiredScopes)
	if len(current) == 0 {
		return fmt.Errorf("GitHub token is missing required access for %s; run: gh auth refresh -s %s", strings.Join(requiredScopes, ","), strings.Join(requiredScopes, ","))
	}
	if len(missing) > 0 {
		return fmt.Errorf("GitHub token scopes are missing %s (current scopes: %s). Run: gh auth refresh -s %s", strings.Join(missing, ","), strings.Join(current, ","), strings.Join(requiredScopes, ","))
	}
	return fmt.Errorf("GitHub API returned %d despite required scopes %s; verify org access and token permissions", resp.StatusCode, strings.Join(requiredScopes, ","))
}

func parseScopes(raw string) []string {
	if raw == "" {
		return nil
	}
	var scopes []string
	for _, scope := range strings.Split(raw, ",") {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			scopes = append(scopes, scope)
		}
	}
	sort.Strings(scopes)
	return scopes
}

func missingScopes(current []string, required []string) []string {
	have := make(map[string]bool, len(current))
	for _, scope := range current {
		have[scope] = true
	}
	var missing []string
	for _, scope := range required {
		if !have[scope] {
			missing = append(missing, scope)
		}
	}
	return missing
}

func normalizeRepoName(org string, repo string) string {
	repo = strings.TrimSpace(repo)
	if strings.Contains(repo, "/") {
		return repo
	}
	return org + "/" + repo
}

func verbosef(cfg DiscoverConfig, format string, args ...any) {
	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}
