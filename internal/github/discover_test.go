package github

import (
	"reflect"
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
