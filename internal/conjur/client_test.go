package conjur

import "testing"

func TestTenantAPIBaseURL(t *testing.T) {
	got := tenantAPIBaseURL("mytenant")
	want := "https://mytenant.secretsmgr.cyberark.cloud/api"

	if got != want {
		t.Fatalf("tenantAPIBaseURL() = %q, want %q", got, want)
	}
}

func TestNormalizeConjurURLPreservesProvidedBase(t *testing.T) {
	tests := map[string]string{
		"https://conjur.example.com":     "https://conjur.example.com",
		"https://conjur.example.com/":    "https://conjur.example.com",
		"https://conjur.example.com/api": "https://conjur.example.com/api",
	}

	for input, wantAPI := range tests {
		_, gotAPI, err := normalizeConjurURL(input)
		if err != nil {
			t.Fatalf("normalizeConjurURL(%q) error: %v", input, err)
		}
		if gotAPI != wantAPI {
			t.Fatalf("normalizeConjurURL(%q) api = %q, want %q", input, gotAPI, wantAPI)
		}
	}
}

func TestClientAPIURLStripsGeneratedAPIPrefixForSaaSBase(t *testing.T) {
	client := &Client{
		apiBaseURL:     tenantAPIBaseURL("mytenant"),
		stripAPIPrefix: true,
	}

	tests := map[string]string{
		"/api/authenticators":          "https://mytenant.secretsmgr.cyberark.cloud/api/authenticators",
		"api/authenticators":           "https://mytenant.secretsmgr.cyberark.cloud/api/authenticators",
		"/api/authenticators/name":     "https://mytenant.secretsmgr.cyberark.cloud/api/authenticators/name",
		"/api/groups/group/members":    "https://mytenant.secretsmgr.cyberark.cloud/api/groups/group/members",
		"/policies/conjur/policy/root": "https://mytenant.secretsmgr.cyberark.cloud/api/policies/conjur/policy/root",
	}

	for input, want := range tests {
		if got := client.apiURL(input); got != want {
			t.Fatalf("apiURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestClientAPIURLPreservesGeneratedPathsForSelfHostedBase(t *testing.T) {
	client := &Client{
		apiBaseURL:     "https://conjur.example.com",
		stripAPIPrefix: false,
	}

	tests := map[string]string{
		"/api/authenticators":          "https://conjur.example.com/api/authenticators",
		"api/authenticators":           "https://conjur.example.com/api/authenticators",
		"/policies/conjur/policy/root": "https://conjur.example.com/policies/conjur/policy/root",
	}

	for input, want := range tests {
		if got := client.apiURL(input); got != want {
			t.Fatalf("apiURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeAPIPathStripsForSaaS(t *testing.T) {
	tests := map[string]string{
		"":                        "",
		"/api":                    "",
		"/api/authenticators":     "/authenticators",
		"api/authenticators":      "/authenticators",
		"/policies/conjur/policy": "/policies/conjur/policy",
		"policies/conjur/policy":  "/policies/conjur/policy",
	}

	for input, want := range tests {
		if got := normalizeAPIPath(input, true); got != want {
			t.Fatalf("normalizeAPIPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeAPIPathPreservesForSelfHosted(t *testing.T) {
	tests := map[string]string{
		"":                        "",
		"/api":                    "/api",
		"/api/authenticators":     "/api/authenticators",
		"api/authenticators":      "/api/authenticators",
		"/policies/conjur/policy": "/policies/conjur/policy",
		"policies/conjur/policy":  "/policies/conjur/policy",
	}

	for input, want := range tests {
		if got := normalizeAPIPath(input, false); got != want {
			t.Fatalf("normalizeAPIPath(%q) = %q, want %q", input, got, want)
		}
	}
}
