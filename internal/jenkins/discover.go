// Package jenkins handles Jenkins discovery for the conjur-onboard CLI.
package jenkins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultMaxDepth = 6

type DiscoverConfig struct {
	JenkinsURL   string
	Username     string
	Token        string
	JobsFromFile string
	MaxDepth     int
	Verbose      bool
}

type DiscoveryResult struct {
	Platform       string            `json:"platform"`
	JenkinsURL     string            `json:"jenkins_url"`
	Controller     string            `json:"controller"`
	ControllerSlug string            `json:"controller_slug"`
	Version        string            `json:"version,omitempty"`
	PluginVersion  string            `json:"plugin_version,omitempty"`
	OIDCIssuer     string            `json:"oidc_issuer"`
	JWKSURI        string            `json:"jwks_uri"`
	Jobs           []JobInfo         `json:"jobs"`
	Source         string            `json:"source"`
	Warnings       []string          `json:"warnings,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	DiscoveredAt   string            `json:"discovered_at"`
}

type JobInfo struct {
	Name     string            `json:"name"`
	FullName string            `json:"full_name"`
	URL      string            `json:"url,omitempty"`
	Type     string            `json:"type"`
	Class    string            `json:"class,omitempty"`
	Parent   string            `json:"parent,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type apiJob struct {
	Name     string   `json:"name"`
	FullName string   `json:"fullName"`
	URL      string   `json:"url"`
	Class    string   `json:"_class"`
	Jobs     []apiJob `json:"jobs"`
}

func Discover(ctx context.Context, cfg DiscoverConfig) (*DiscoveryResult, error) {
	baseURL, err := normalizeJenkinsURL(cfg.JenkinsURL)
	if err != nil {
		return nil, err
	}
	controller := controllerName(baseURL)
	result := &DiscoveryResult{
		Platform:       "jenkins",
		JenkinsURL:     baseURL,
		Controller:     controller,
		ControllerSlug: SafeName(controller),
		OIDCIssuer:     baseURL,
		JWKSURI:        baseURL + "/jwtauth/conjur-jwk-set",
		Metadata:       map[string]string{},
		DiscoveredAt:   time.Now().UTC().Format(time.RFC3339),
	}

	if cfg.JobsFromFile != "" {
		jobs, err := LoadJobsFile(cfg.JobsFromFile)
		if err != nil {
			return nil, err
		}
		result.Jobs = jobs
		result.Source = "jobs-from-file"
		result.Metadata["jobs_file"] = cfg.JobsFromFile
		return result, nil
	}

	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = defaultMaxDepth
	}
	client := &http.Client{Timeout: 30 * time.Second}
	jobs, version, err := fetchJobs(ctx, client, cfg, baseURL)
	if err != nil {
		return nil, err
	}
	result.Jobs = jobs
	result.Version = version
	result.Source = "api"
	result.Metadata["max_depth"] = fmt.Sprintf("%d", cfg.MaxDepth)

	pluginVersion, warning := fetchConjurPluginVersion(ctx, client, cfg, baseURL)
	if pluginVersion != "" {
		result.PluginVersion = pluginVersion
	}
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	return result, nil
}

func LoadDiscovery(workDir string) (*DiscoveryResult, error) {
	path := filepath.Join(workDir, "discovery.json")
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

func LoadJobsFile(path string) ([]JobInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading jobs file %s: %w", path, err)
	}
	var jobs []JobInfo
	for lineNo, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fullName, typ, ok := strings.Cut(line, "|")
		fullName = strings.TrimSpace(fullName)
		typ = strings.TrimSpace(typ)
		if fullName == "" {
			return nil, fmt.Errorf("%s:%d missing Jenkins full name", path, lineNo+1)
		}
		if !ok || typ == "" {
			typ = inferTypeFromFullName(fullName)
		}
		jobs = append(jobs, JobInfo{
			Name:     leafName(fullName),
			FullName: fullName,
			Type:     normalizeJobType(typ),
			Parent:   parentName(fullName),
		})
	}
	sortJobs(jobs)
	return jobs, nil
}

func fetchJobs(ctx context.Context, client *http.Client, cfg DiscoverConfig, baseURL string) ([]JobInfo, string, error) {
	apiURL := baseURL + "/api/json?tree=" + url.QueryEscape("jobs["+jobTree(cfg.MaxDepth)+"]")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, "", err
	}
	setAuth(req, cfg)

	if cfg.Verbose {
		fmt.Printf("  [jenkins] GET %s\n", apiURL)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("GET Jenkins jobs: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading Jenkins jobs response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, "", fmt.Errorf("Jenkins API returned HTTP %d while listing jobs; provide --username and JENKINS_API_TOKEN with read access", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("Jenkins API returned HTTP %d while listing jobs: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Jobs []apiJob `json:"jobs"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", fmt.Errorf("parsing Jenkins jobs response: %w", err)
	}
	var jobs []JobInfo
	flattenJobs(parsed.Jobs, &jobs)
	sortJobs(jobs)
	return jobs, resp.Header.Get("X-Jenkins"), nil
}

func fetchConjurPluginVersion(ctx context.Context, client *http.Client, cfg DiscoverConfig, baseURL string) (string, string) {
	apiURL := baseURL + "/pluginManager/api/json?depth=1&tree=" + url.QueryEscape("plugins[shortName,version]")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Sprintf("could not build Jenkins plugin discovery request: %v", err)
	}
	setAuth(req, cfg)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Sprintf("could not detect Conjur Jenkins plugin version: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Sprintf("could not read Jenkins plugin discovery response: %v", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Sprintf("could not detect Conjur Jenkins plugin version: Jenkins API returned HTTP %d", resp.StatusCode)
	}
	var parsed struct {
		Plugins []struct {
			ShortName string `json:"shortName"`
			Version   string `json:"version"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Sprintf("could not parse Jenkins plugin discovery response: %v", err)
	}
	for _, plugin := range parsed.Plugins {
		if plugin.ShortName == "conjur-credentials" {
			return plugin.Version, ""
		}
	}
	return "", "Conjur Jenkins plugin was not found in pluginManager output"
}

func flattenJobs(items []apiJob, dest *[]JobInfo) {
	for _, item := range items {
		fullName := item.FullName
		if fullName == "" {
			fullName = item.Name
		}
		if fullName != "" {
			*dest = append(*dest, JobInfo{
				Name:     firstNonEmpty(item.Name, leafName(fullName)),
				FullName: fullName,
				URL:      item.URL,
				Type:     inferJobType(item.Class),
				Class:    item.Class,
				Parent:   parentName(fullName),
			})
		}
		flattenJobs(item.Jobs, dest)
	}
}

func jobTree(depth int) string {
	fields := "name,fullName,url,_class"
	if depth <= 1 {
		return fields
	}
	return fields + ",jobs[" + jobTree(depth-1) + "]"
}

func normalizeJenkinsURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("--url is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parsing Jenkins URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("Jenkins URL must include scheme and host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func setAuth(req *http.Request, cfg DiscoverConfig) {
	if cfg.Username != "" && cfg.Token != "" {
		req.SetBasicAuth(cfg.Username, cfg.Token)
	}
}

func sortJobs(jobs []JobInfo) {
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].FullName < jobs[j].FullName
	})
}

func inferJobType(class string) string {
	class = strings.ToLower(class)
	switch {
	case strings.Contains(class, "organizationfolder"):
		return "folder"
	case strings.Contains(class, "folder"):
		return "folder"
	case strings.Contains(class, "workflowmultibranchproject"):
		return "multibranch"
	case strings.Contains(class, "workflowjob"):
		return "pipeline"
	case strings.Contains(class, "freestyleproject"), strings.Contains(class, "matrixproject"):
		return "job"
	default:
		return "job"
	}
}

func inferTypeFromFullName(fullName string) string {
	if fullName == "GlobalCredentials" {
		return "global"
	}
	return "scope"
}

func normalizeJobType(typ string) string {
	typ = strings.ToLower(strings.TrimSpace(typ))
	switch typ {
	case "global", "folder", "multibranch", "pipeline", "job", "scope":
		return typ
	default:
		return "scope"
	}
}

func controllerName(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return "jenkins"
	}
	return parsed.Host
}

func parentName(fullName string) string {
	fullName = strings.Trim(fullName, "/")
	idx := strings.LastIndex(fullName, "/")
	if idx <= 0 {
		return ""
	}
	return fullName[:idx]
}

func leafName(fullName string) string {
	fullName = strings.Trim(fullName, "/")
	idx := strings.LastIndex(fullName, "/")
	if idx < 0 {
		return fullName
	}
	return fullName[idx+1:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
