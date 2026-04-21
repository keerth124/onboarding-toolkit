package conjur

import (
	"fmt"
	"regexp"
	"strings"

	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
)

// GenerateConfig holds all inputs for artifact generation.
type GenerateConfig struct {
	Discovery     *ghdisc.DiscoveryResult
	Tenant        string
	Audience      string
	CreateEnabled bool
	WorkDir       string
	Verbose       bool
	DryRun        bool
}

// GenerateResult summarizes what was generated.
type GenerateResult struct {
	AuthenticatorName string
	WorkloadCount     int
}

// sanitizeName returns a string safe for use as a Conjur resource name.
// Allowed characters: A-Z a-z 0-9 - _
var nonSafeRE = regexp.MustCompile(`[^A-Za-z0-9\-_]`)

func sanitizeName(s string) string {
	return nonSafeRE.ReplaceAllString(strings.ToLower(s), "-")
}

// authenticatorName builds the deterministic authenticator name from the org.
func authenticatorName(org string) string {
	return "github-" + sanitizeName(org)
}

// identityPath returns the policy branch where workloads live.
func identityPath(org string) string {
	return "data/github-apps/" + sanitizeName(org)
}

// appsGroupID returns the URL-encoded apps group identifier.
func appsGroupID(authnName string) string {
	raw := fmt.Sprintf("conjur/authn-jwt/%s/apps", authnName)
	// URL-encode the slashes for use in the API path.
	return strings.ReplaceAll(raw, "/", "%2F")
}

// workloadID returns the workload host ID for a given repo (and optionally environment).
func workloadID(identPath, repoFullName string, env string) string {
	if env != "" {
		return identPath + "/" + repoFullName + "/" + env
	}
	return identPath + "/" + repoFullName
}

// hasEnvironments returns true if any repo in the discovery result has environments configured.
func hasEnvironments(repos []ghdisc.RepoInfo) bool {
	for _, r := range repos {
		if len(r.Environments) > 0 {
			return true
		}
	}
	return false
}

// enforcedClaims returns the enforced_claims list based on what the discovery found.
func enforcedClaims(repos []ghdisc.RepoInfo) []string {
	if hasEnvironments(repos) {
		return []string{"environment"}
	}
	return nil
}
