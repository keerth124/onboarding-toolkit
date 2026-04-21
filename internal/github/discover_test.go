package github

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestParseScopes(t *testing.T) {
	got := parseScopes("repo, read:org, workflow")
	want := []string{"read:org", "repo", "workflow"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseScopes() = %#v, want %#v", got, want)
	}
}

func TestMissingScopes(t *testing.T) {
	got := missingScopes([]string{"repo"}, []string{"repo", "read:org"})
	want := []string{"read:org"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missingScopes() = %#v, want %#v", got, want)
	}
}

func TestNormalizeRepoName(t *testing.T) {
	tests := map[string]string{
		"api-service": "acme/api-service",
		"acme/web":    "acme/web",
	}

	for input, want := range tests {
		if got := normalizeRepoName("acme", input); got != want {
			t.Fatalf("normalizeRepoName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGetOrgInfoFallsBackToUserOwner(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/orgs/keerth124":
			return jsonResponse(http.StatusNotFound, `{}`), nil
		case "/users/keerth124":
			return jsonResponse(http.StatusOK, `{"id":123,"login":"keerth124","name":"Keerth","node_id":"U_123","public_repos":7,"type":"User"}`), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	})}

	got, err := getOrgInfo(context.Background(), client, DiscoverConfig{Org: "keerth124"})
	if err != nil {
		t.Fatal(err)
	}

	if got.Login != "keerth124" {
		t.Fatalf("Login = %q, want keerth124", got.Login)
	}
	if got.AccountType != "User" {
		t.Fatalf("AccountType = %q, want User", got.AccountType)
	}
	if got.PublicRepos != 7 {
		t.Fatalf("PublicRepos = %d, want 7", got.PublicRepos)
	}
}

func TestListReposUsesUserEndpointForUserOwner(t *testing.T) {
	var paths []string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		if req.URL.Path != "/users/keerth124/repos" {
			t.Fatalf("unexpected path %s", req.URL.Path)
		}
		return jsonResponse(http.StatusOK, `[{"name":"onboarding-toolkit","full_name":"keerth124/onboarding-toolkit","default_branch":"main","visibility":"public","archived":false}]`), nil
	})}

	repos, err := listRepos(context.Background(), client, DiscoverConfig{Org: "keerth124"}, OrgInfo{AccountType: "User"})
	if err != nil {
		t.Fatal(err)
	}

	if len(paths) != 1 {
		t.Fatalf("paths = %#v, want one request", paths)
	}
	if len(repos) != 1 || repos[0].FullName != "keerth124/onboarding-toolkit" {
		t.Fatalf("repos = %#v, want keerth124/onboarding-toolkit", repos)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
