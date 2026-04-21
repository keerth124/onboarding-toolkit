package github

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultTokenAppProperty = "repository"
)

var supportedIdentityClaims = map[string]bool{
	"repository":       true,
	"repository_owner": true,
	"workflow_ref":     true,
}

var supportedEnforcedClaims = map[string]bool{
	"environment":  true,
	"workflow_ref": true,
}

type ClaimAnalysis struct {
	Platform           string         `json:"platform"`
	Mode               string         `json:"mode"`
	Repository         string         `json:"repository,omitempty"`
	Recommended        []string       `json:"recommended"`
	SelectedClaims     ClaimSelection `json:"selected_claims"`
	AvailableClaims    []ClaimRecord  `json:"available_claims"`
	SecurityWarnings   []string       `json:"security_warnings,omitempty"`
	SecurityNotes      []string       `json:"security_notes,omitempty"`
	ImplementationNote string         `json:"implementation_note,omitempty"`
}

type ClaimSelection struct {
	TokenAppProperty string   `json:"token_app_property"`
	EnforcedClaims   []string `json:"enforced_claims"`
}

type ClaimRecord struct {
	Name           string `json:"name"`
	ExampleValue   string `json:"example_value,omitempty"`
	Classification string `json:"classification"`
	Recommended    bool   `json:"recommended"`
	Explanation    string `json:"explanation"`
}

func BuildDefaultClaimAnalysis(disc *DiscoveryResult) ClaimAnalysis {
	repo := ""
	if disc != nil && len(disc.Repos) > 0 {
		repo = disc.Repos[0].FullName
	}
	return BuildSyntheticClaimAnalysis(repo, "", ClaimSelection{
		TokenAppProperty: DefaultTokenAppProperty,
		EnforcedClaims:   nil,
	})
}

func BuildSyntheticClaimAnalysis(repo string, environment string, selection ClaimSelection) ClaimAnalysis {
	if selection.TokenAppProperty == "" {
		selection.TokenAppProperty = DefaultTokenAppProperty
	}
	selection.EnforcedClaims = normalizeClaims(selection.EnforcedClaims)

	owner := ""
	if strings.Contains(repo, "/") {
		owner = strings.SplitN(repo, "/", 2)[0]
	}
	envValue := environment
	if envValue == "" {
		envValue = "production"
	}

	return ClaimAnalysis{
		Platform:    "github",
		Mode:        "synthetic",
		Repository:  repo,
		Recommended: []string{"repository"},
		SelectedClaims: ClaimSelection{
			TokenAppProperty: selection.TokenAppProperty,
			EnforcedClaims:   selection.EnforcedClaims,
		},
		AvailableClaims: []ClaimRecord{
			{
				Name:           "iss",
				ExampleValue:   "https://token.actions.githubusercontent.com",
				Classification: "metadata",
				Recommended:    false,
				Explanation:    "Issuer used by Conjur to verify the JWT source.",
			},
			{
				Name:           "aud",
				ExampleValue:   "conjur-cloud",
				Classification: "scope",
				Recommended:    false,
				Explanation:    "Audience expected by the Conjur authenticator.",
			},
			{
				Name:           "repository",
				ExampleValue:   repo,
				Classification: "identity-strong",
				Recommended:    true,
				Explanation:    "Recommended primary identity claim; binds access to one repository.",
			},
			{
				Name:           "repository_owner",
				ExampleValue:   owner,
				Classification: "identity-weak",
				Recommended:    false,
				Explanation:    "Too broad by itself because every repository in the owner scope can share it.",
			},
			{
				Name:           "environment",
				ExampleValue:   envValue,
				Classification: "scope",
				Recommended:    false,
				Explanation:    "Useful for protected deployment environments when paired with a compatible identity strategy.",
			},
			{
				Name:           "workflow_ref",
				ExampleValue:   repo + "/.github/workflows/deploy.yml@refs/heads/main",
				Classification: "scope",
				Recommended:    false,
				Explanation:    "Precise but brittle because workflow file moves and branch changes alter the claim.",
			},
			{
				Name:           "run_id",
				ExampleValue:   "1234567890",
				Classification: "ephemeral",
				Recommended:    false,
				Explanation:    "Changes every run and should not be used for stable workload identity.",
			},
		},
		SecurityWarnings: securityWarnings(selection),
		SecurityNotes: []string{
			"Use repository as the default identity claim for GitHub Actions.",
			"Do not use repository_owner by itself unless every repository in the organization should share the same workload identity.",
			"Environment scoping can improve separation, but it requires a compatible identity strategy before being enforced.",
		},
		ImplementationNote: "Live token inspection and interactive claim selection are planned follow-up work. The MVP generator supports repository as the token_app_property and no enforced claims.",
	}
}

func ParseClaimSelection(tokenAppProperty string, enforcedClaimsCSV string) (ClaimSelection, error) {
	selection := ClaimSelection{
		TokenAppProperty: strings.TrimSpace(tokenAppProperty),
		EnforcedClaims:   splitClaims(enforcedClaimsCSV),
	}
	if selection.TokenAppProperty == "" {
		selection.TokenAppProperty = DefaultTokenAppProperty
	}
	selection.EnforcedClaims = normalizeClaims(selection.EnforcedClaims)
	if err := ValidateKnownClaims(selection); err != nil {
		return ClaimSelection{}, err
	}
	return selection, nil
}

func LoadClaimAnalysis(workDir string) (ClaimAnalysis, bool, error) {
	path := filepath.Join(workDir, "claims-analysis.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ClaimAnalysis{}, false, nil
	}
	if err != nil {
		return ClaimAnalysis{}, false, fmt.Errorf("reading %s: %w", path, err)
	}

	var analysis ClaimAnalysis
	if err := json.Unmarshal(data, &analysis); err != nil {
		return ClaimAnalysis{}, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	return analysis, true, nil
}

func ValidateKnownClaims(selection ClaimSelection) error {
	if !supportedIdentityClaims[selection.TokenAppProperty] {
		return fmt.Errorf("unsupported GitHub token_app_property %q", selection.TokenAppProperty)
	}
	for _, claim := range selection.EnforcedClaims {
		if !supportedEnforcedClaims[claim] {
			return fmt.Errorf("unsupported GitHub enforced claim %q", claim)
		}
	}
	return nil
}

func ValidateGeneratorSupportedSelection(selection ClaimSelection) error {
	if err := ValidateKnownClaims(selection); err != nil {
		return err
	}
	if selection.TokenAppProperty != DefaultTokenAppProperty {
		return fmt.Errorf("GitHub generator currently supports token_app_property %q only; rerun inspect with --token-app-property repository", DefaultTokenAppProperty)
	}
	if len(selection.EnforcedClaims) > 0 {
		return fmt.Errorf("GitHub generator does not yet support enforced claims %v; rerun inspect without --enforced-claims", selection.EnforcedClaims)
	}
	return nil
}

func splitClaims(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	return strings.Split(csv, ",")
}

func normalizeClaims(claims []string) []string {
	seen := map[string]bool{}
	var normalized []string
	for _, claim := range claims {
		claim = strings.TrimSpace(claim)
		if claim == "" || seen[claim] {
			continue
		}
		seen[claim] = true
		normalized = append(normalized, claim)
	}
	sort.Strings(normalized)
	return normalized
}

func securityWarnings(selection ClaimSelection) []string {
	var warnings []string
	if selection.TokenAppProperty == "repository_owner" {
		warnings = append(warnings, "Selecting repository_owner grants every repository in that owner scope the same workload identity.")
	}
	if selection.TokenAppProperty == "workflow_ref" {
		warnings = append(warnings, "Selecting workflow_ref is precise but brittle because workflow file moves and branch changes alter the identity.")
	}
	for _, claim := range selection.EnforcedClaims {
		if claim == "environment" {
			warnings = append(warnings, "Environment scoping should be validated with a live token before production use.")
		}
	}
	return warnings
}
