// Package platform defines platform-neutral contracts used by onboarding
// adapters and Conjur artifact generation.
package platform

import (
	"context"
	"fmt"
	"strings"
)

// Descriptor identifies a supported CI/CD or workload platform.
type Descriptor struct {
	ID          string
	DisplayName string
	Description string
}

// Validate returns an error when the descriptor is missing fields required for
// command registration, artifact generation, or user-facing output.
func (d Descriptor) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("platform id is required")
	}
	if strings.TrimSpace(d.DisplayName) == "" {
		return fmt.Errorf("platform display name is required")
	}
	return nil
}

// DiscoveryInput carries common discovery settings. Platform adapters can use
// Options for platform-specific values such as org, group, project, or URL.
type DiscoveryInput struct {
	WorkDir string
	Token   string
	Verbose bool
	Options map[string]string
}

// Discovery is the normalized discovery output passed from a platform adapter
// into claim analysis and Conjur artifact generation.
type Discovery struct {
	Platform     Descriptor        `json:"platform"`
	Scope        Scope             `json:"scope"`
	OIDCProvider OIDCProvider      `json:"oidc_provider,omitempty"`
	Resources    []Resource        `json:"resources"`
	Warnings     []string          `json:"warnings,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	DiscoveredAt string            `json:"discovered_at,omitempty"`
}

// Scope names the platform boundary being onboarded, such as a GitHub org, a
// GitLab group, or a Jenkins folder root.
type Scope struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name,omitempty"`
	Type        string            `json:"type,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// OIDCProvider describes a JWT issuer discovered or derived for a platform.
type OIDCProvider struct {
	Issuer  string `json:"issuer,omitempty"`
	JWKSURI string `json:"jwks_uri,omitempty"`
}

// Resource is one discovered platform object that can map to one or more
// Conjur workloads. Examples include repositories, projects, jobs, service
// connections, or runner identities.
type Resource struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	FullName      string            `json:"full_name,omitempty"`
	Type          string            `json:"type,omitempty"`
	Parent        string            `json:"parent,omitempty"`
	DefaultBranch string            `json:"default_branch,omitempty"`
	Visibility    string            `json:"visibility,omitempty"`
	Environments  []string          `json:"environments,omitempty"`
	Archived      bool              `json:"archived,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// ClaimInspectionInput identifies the sample context used to inspect or
// synthesize claims for a platform.
type ClaimInspectionInput struct {
	Discovery       *Discovery
	Resource        Resource
	Environment     string
	Mode            string
	ClaimSelection  ClaimSelection
	PlatformOptions map[string]string
}

// ClaimSelection records the claim strategy selected for a JWT authenticator.
type ClaimSelection struct {
	TokenAppProperty string   `json:"token_app_property"`
	EnforcedClaims   []string `json:"enforced_claims,omitempty"`
}

// ClaimAnalysis records available claims, recommended defaults, and the
// selected claim strategy for review and generation.
type ClaimAnalysis struct {
	Platform           Descriptor     `json:"platform"`
	Mode               string         `json:"mode"`
	Subject            string         `json:"subject,omitempty"`
	Recommended        []string       `json:"recommended"`
	SelectedClaims     ClaimSelection `json:"selected_claims"`
	AvailableClaims    []ClaimRecord  `json:"available_claims"`
	SecurityWarnings   []string       `json:"security_warnings,omitempty"`
	SecurityNotes      []string       `json:"security_notes,omitempty"`
	ImplementationNote string         `json:"implementation_note,omitempty"`
}

// ClaimRecord describes one available platform identity claim.
type ClaimRecord struct {
	Name           string `json:"name"`
	ExampleValue   string `json:"example_value,omitempty"`
	Classification string `json:"classification"`
	Recommended    bool   `json:"recommended"`
	Explanation    string `json:"explanation"`
}

// GenerationInput contains the platform-neutral inputs needed to generate
// Conjur API artifacts.
type GenerationInput struct {
	Discovery         *Discovery
	Claims            ClaimAnalysis
	Tenant            string
	ConjurURL         string
	ConjurTarget      string
	Audience          string
	CreateEnabled     bool
	WorkDir           string
	ProvisioningMode  string
	AuthenticatorName string
	Verbose           bool
	DryRun            bool
}

// Authenticator describes the Conjur authenticator a platform adapter needs.
type Authenticator struct {
	Type             string
	Subtype          string
	Name             string
	Enabled          bool
	Issuer           string
	JWKSURI          string
	Audience         string
	IdentityPath     string
	TokenAppProperty string
	EnforcedClaims   []string
	Metadata         map[string]string
}

// Workload represents a single Conjur workload identity to create.
type Workload struct {
	FullPath    string            `json:"full_path"`
	HostID      string            `json:"host_id"`
	DisplayName string            `json:"display_name,omitempty"`
	SourceID    string            `json:"source_id,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// IntegrationArtifact is a generated platform-side file, such as a pipeline
// snippet, workflow, Jenkinsfile fragment, or setup guide.
type IntegrationArtifact struct {
	Path        string
	Content     string
	Description string
	Sensitive   bool
}

// IntegrationInput contains all values a platform adapter needs to produce
// consumer-side integration artifacts.
type IntegrationInput struct {
	GenerationInput GenerationInput
	Authenticator   Authenticator
	Workloads       []Workload
	AppsGroupID     string
}

// NextStepsInput contains values needed for a platform-specific walkthrough.
type NextStepsInput struct {
	GenerationInput GenerationInput
	Authenticator   Authenticator
	Workloads       []Workload
	AppsGroupID     string
}

// Adapter is the minimum contract a platform must satisfy to plug into the
// common onboarding flow.
type Adapter interface {
	Descriptor() Descriptor
	Discover(ctx context.Context, input DiscoveryInput) (*Discovery, error)
	InspectClaims(ctx context.Context, input ClaimInspectionInput) (ClaimAnalysis, error)
	DefaultClaimSelection(discovery *Discovery) ClaimSelection
	Authenticator(input GenerationInput) (Authenticator, error)
	Workloads(input GenerationInput, authenticator Authenticator) ([]Workload, error)
	IntegrationArtifacts(input IntegrationInput) ([]IntegrationArtifact, error)
	NextSteps(input NextStepsInput) (string, error)
}
