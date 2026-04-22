package jenkins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultTokenAppProperty = "jenkins_full_name"
	DefaultAudience         = "cyberark-conjur"
)

type ClaimAnalysis struct {
	Platform           string         `json:"platform"`
	Mode               string         `json:"mode"`
	JobFullName        string         `json:"job_full_name,omitempty"`
	Recommended        []string       `json:"recommended"`
	SelectedClaims     ClaimSelection `json:"selected_claims"`
	AvailableClaims    []ClaimRecord  `json:"available_claims"`
	SecurityWarnings   []string       `json:"security_warnings,omitempty"`
	SecurityNotes      []string       `json:"security_notes,omitempty"`
	ImplementationNote string         `json:"implementation_note,omitempty"`
}

type ClaimSelection struct {
	TokenAppProperty string   `json:"token_app_property"`
	EnforcedClaims   []string `json:"enforced_claims,omitempty"`
}

type ClaimRecord struct {
	Name           string `json:"name"`
	ExampleValue   string `json:"example_value,omitempty"`
	Classification string `json:"classification"`
	Recommended    bool   `json:"recommended"`
	Explanation    string `json:"explanation"`
}

func BuildDefaultClaimAnalysis(disc *DiscoveryResult) ClaimAnalysis {
	job := "Folder/Deploy"
	if disc != nil && len(disc.Jobs) > 0 {
		job = disc.Jobs[0].FullName
	}
	return BuildSyntheticClaimAnalysis(job, ClaimSelection{
		TokenAppProperty: DefaultTokenAppProperty,
	})
}

func BuildSyntheticClaimAnalysis(jobFullName string, selection ClaimSelection) ClaimAnalysis {
	if strings.TrimSpace(jobFullName) == "" {
		jobFullName = "Folder/Deploy"
	}
	if selection.TokenAppProperty == "" {
		selection.TokenAppProperty = DefaultTokenAppProperty
	}
	selection.EnforcedClaims = normalizeClaims(selection.EnforcedClaims)
	parent := parentName(jobFullName)
	name := leafName(jobFullName)

	return ClaimAnalysis{
		Platform:    "jenkins",
		Mode:        "synthetic",
		JobFullName: jobFullName,
		Recommended: []string{DefaultTokenAppProperty},
		SelectedClaims: ClaimSelection{
			TokenAppProperty: selection.TokenAppProperty,
			EnforcedClaims:   selection.EnforcedClaims,
		},
		AvailableClaims: []ClaimRecord{
			{
				Name:           "iss",
				ExampleValue:   "https://jenkins.example.com",
				Classification: "metadata",
				Recommended:    false,
				Explanation:    "Jenkins controller URL used by Conjur to verify the JWT source.",
			},
			{
				Name:           "aud",
				ExampleValue:   DefaultAudience,
				Classification: "scope",
				Recommended:    false,
				Explanation:    "Audience expected by the Conjur authenticator.",
			},
			{
				Name:           "sub",
				ExampleValue:   jobFullName,
				Classification: "identity-strong",
				Recommended:    false,
				Explanation:    "Subject can mirror Jenkins full name, but plugin configuration may change its format.",
			},
			{
				Name:           "jenkins_full_name",
				ExampleValue:   jobFullName,
				Classification: "identity-strong",
				Recommended:    true,
				Explanation:    "Recommended primary identity claim; maps directly to the Jenkins credential scope full name.",
			},
			{
				Name:           "jenkins_name",
				ExampleValue:   name,
				Classification: "identity-weak",
				Recommended:    false,
				Explanation:    "Job or folder leaf name; weak by itself because names can repeat under different parents.",
			},
			{
				Name:           "jenkins_parent_full_name",
				ExampleValue:   parent,
				Classification: "scope",
				Recommended:    false,
				Explanation:    "Parent folder path; useful for review but not needed when jenkins_full_name is the primary identity.",
			},
			{
				Name:           "jenkins_parent_name",
				ExampleValue:   leafName(parent),
				Classification: "metadata",
				Recommended:    false,
				Explanation:    "Parent leaf name; too ambiguous for workload identity.",
			},
			{
				Name:           "exp",
				ExampleValue:   "1710003600",
				Classification: "ephemeral",
				Recommended:    false,
				Explanation:    "Token expiration time and not stable enough for workload identity.",
			},
		},
		SecurityWarnings: securityWarnings(selection),
		SecurityNotes: []string{
			"Use jenkins_full_name as the default identity claim for Jenkins plugin-issued JWTs.",
			"Create workloads for the Jenkins credential scope that should receive secret access: global, folder, multibranch parent, or leaf job.",
			"Do not use jenkins_name alone because the same leaf name can exist in multiple folders.",
		},
		ImplementationNote: "The Jenkins generator supports jenkins_full_name as the token_app_property and no enforced claims in this first slice.",
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
	switch selection.TokenAppProperty {
	case "jenkins_full_name", "sub":
	default:
		return fmt.Errorf("unsupported Jenkins token_app_property %q", selection.TokenAppProperty)
	}
	for _, claim := range selection.EnforcedClaims {
		switch claim {
		case "jenkins_parent_full_name", "jenkins_name":
		default:
			return fmt.Errorf("unsupported Jenkins enforced claim %q", claim)
		}
	}
	return nil
}

func ValidateGeneratorSupportedSelection(selection ClaimSelection) error {
	if err := ValidateKnownClaims(selection); err != nil {
		return err
	}
	if selection.TokenAppProperty != DefaultTokenAppProperty {
		return fmt.Errorf("Jenkins generator currently supports token_app_property %q only; rerun inspect with --token-app-property %s", DefaultTokenAppProperty, DefaultTokenAppProperty)
	}
	if len(selection.EnforcedClaims) > 0 {
		return fmt.Errorf("Jenkins generator does not yet support enforced claims %v; rerun inspect without --enforced-claims", selection.EnforcedClaims)
	}
	return nil
}

func securityWarnings(selection ClaimSelection) []string {
	var warnings []string
	if selection.TokenAppProperty == "jenkins_name" {
		warnings = append(warnings, "jenkins_name is only the leaf name and can collide across folders; prefer jenkins_full_name.")
	}
	for _, claim := range selection.EnforcedClaims {
		if claim == "jenkins_name" {
			warnings = append(warnings, "jenkins_name as an enforced claim does not add folder-level separation.")
		}
	}
	return warnings
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
