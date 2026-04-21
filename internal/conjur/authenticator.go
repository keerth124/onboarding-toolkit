package conjur

import (
	"fmt"
	"path/filepath"

	"github.com/cyberark/conjur-onboard/internal/core"
	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
)

// AuthenticatorIdentity is the JWT identity binding section of the authenticator body.
type AuthenticatorIdentity struct {
	TokenAppProperty string   `json:"token_app_property"`
	IdentityPath     string   `json:"identity_path"`
	EnforcedClaims   []string `json:"enforced_claims,omitempty"`
}

// AuthenticatorData contains JWKS and identity configuration for JWT authenticators.
type AuthenticatorData struct {
	JWKSUri  string                `json:"jwks_uri"`
	Issuer   string                `json:"issuer"`
	Audience string                `json:"audience"`
	Identity AuthenticatorIdentity `json:"identity"`
}

// AuthenticatorBody is the body for POST /api/authenticators.
type AuthenticatorBody struct {
	Type    string            `json:"type"`
	Subtype string            `json:"subtype"`
	Name    string            `json:"name"`
	Enabled bool              `json:"enabled"`
	Data    AuthenticatorData `json:"data"`
}

// buildAuthenticatorBody constructs the deterministic authenticator request body.
func buildAuthenticatorBody(disc *ghdisc.DiscoveryResult, cfg GenerateConfig, selection ghdisc.ClaimSelection, authnName string) AuthenticatorBody {
	identPath := identityPath(disc.Org)

	return AuthenticatorBody{
		Type:    "jwt",
		Subtype: "github_actions",
		Name:    authnName,
		Enabled: cfg.CreateEnabled,
		Data: AuthenticatorData{
			JWKSUri:  disc.JWKSUri,
			Issuer:   disc.OIDCIssuer,
			Audience: cfg.Audience,
			Identity: AuthenticatorIdentity{
				TokenAppProperty: selection.TokenAppProperty,
				IdentityPath:     identPath,
				EnforcedClaims:   selection.EnforcedClaims,
			},
		},
	}
}

// writeAuthenticatorArtifact writes 01-create-authenticator.json.
func writeAuthenticatorArtifact(disc *ghdisc.DiscoveryResult, cfg GenerateConfig, selection ghdisc.ClaimSelection, authnName string) (AuthenticatorBody, error) {
	body := buildAuthenticatorBody(disc, cfg, selection, authnName)
	destDir := filepath.Join(cfg.WorkDir, "api")
	if err := core.WriteJSON(destDir, "01-create-authenticator.json", body); err != nil {
		return body, fmt.Errorf("writing authenticator artifact: %w", err)
	}
	return body, nil
}
