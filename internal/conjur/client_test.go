package conjur

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestNewHTTPClientVerifiesTLSByDefault(t *testing.T) {
	client := newHTTPClient(false)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	if transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("TLS verification was disabled by default")
	}
}

func TestNewHTTPClientCanSkipTLSVerification(t *testing.T) {
	client := newHTTPClient(true)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("TLS verification was not disabled")
	}
}

func TestAuthenticateFailureIncludesConjurResponseAndHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
	}))
	defer server.Close()

	_, err := authenticate(server.Client(), server.URL, "conjur", "admin", "not-an-api-key", false)
	if err == nil {
		t.Fatal("expected authentication error")
	}
	msg := err.Error()
	for _, want := range []string{"HTTP 401 Unauthorized", `{"error":"invalid credentials"}`, "not the UI password"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("err = %q, want %q", msg, want)
		}
	}
}

func TestAuthHTTPErrorReportsEmptyResponse(t *testing.T) {
	got := authHTTPError(http.StatusUnauthorized, nil)
	if !strings.Contains(got, "<empty response>") {
		t.Fatalf("authHTTPError() = %q, want empty response marker", got)
	}
}

func TestClientAPIURLStripsGeneratedAPIPrefixForSaaSBase(t *testing.T) {
	client := &Client{
		apiBaseURL:     tenantAPIBaseURL("mytenant"),
		stripAPIPrefix: true,
		account:        "conjur",
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

func TestClientAPIURLMapsAuthenticatorPathsForSelfHostedBase(t *testing.T) {
	client := &Client{
		apiBaseURL:     "https://conjur.example.com",
		stripAPIPrefix: false,
		account:        "conjur",
	}

	tests := map[string]string{
		"/api/authenticators":             "https://conjur.example.com/authenticators/conjur",
		"api/authenticators":              "https://conjur.example.com/authenticators/conjur",
		"/api/authenticators/name":        "https://conjur.example.com/authenticators/conjur/name",
		"/authenticators/{account}":       "https://conjur.example.com/authenticators/conjur",
		"/authenticators/{account}/name":  "https://conjur.example.com/authenticators/conjur/name",
		"/policies/{account}/policy/root": "https://conjur.example.com/policies/conjur/policy/root",
		"/policies/conjur/policy/root":    "https://conjur.example.com/policies/conjur/policy/root",
		"/api/groups/group/members":       "https://conjur.example.com/api/groups/group/members",
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
		if got := normalizeAPIPath(input, true, "conjur"); got != want {
			t.Fatalf("normalizeAPIPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeAPIPathMapsAuthenticatorPathsForSelfHosted(t *testing.T) {
	tests := map[string]string{
		"":                              "",
		"/api":                          "/api",
		"/api/authenticators":           "/authenticators/conjur",
		"api/authenticators":            "/authenticators/conjur",
		"/api/authenticators/github":    "/authenticators/conjur/github",
		"/authenticators/{account}":     "/authenticators/conjur",
		"/authenticators/{account}/git": "/authenticators/conjur/git",
		"/policies/{account}/policy":    "/policies/conjur/policy",
		"/policies/conjur/policy":       "/policies/conjur/policy",
		"policies/conjur/policy":        "/policies/conjur/policy",
	}

	for input, want := range tests {
		if got := normalizeAPIPath(input, false, "conjur"); got != want {
			t.Fatalf("normalizeAPIPath(%q) = %q, want %q", input, got, want)
		}
	}
}
