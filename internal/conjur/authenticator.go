package conjur

import (
	"fmt"
	"path/filepath"

	"github.com/cyberark/conjur-onboard/internal/core"
	"github.com/cyberark/conjur-onboard/internal/platform"
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

// buildAuthenticatorBody constructs the deterministic authenticator request
// body from platform-neutral authenticator metadata.
func buildAuthenticatorBody(authn platform.Authenticator) AuthenticatorBody {
	return AuthenticatorBody{
		Type:    authn.Type,
		Subtype: authn.Subtype,
		Name:    authn.Name,
		Enabled: authn.Enabled,
		Data: AuthenticatorData{
			JWKSUri:  authn.JWKSURI,
			Issuer:   authn.Issuer,
			Audience: authn.Audience,
			Identity: AuthenticatorIdentity{
				TokenAppProperty: authn.TokenAppProperty,
				IdentityPath:     authn.IdentityPath,
				EnforcedClaims:   authn.EnforcedClaims,
			},
		},
	}
}

// writeAuthenticatorArtifact writes 01-create-authenticator.json.
func writeAuthenticatorArtifact(authn platform.Authenticator, cfg GenerateConfig) (AuthenticatorBody, error) {
	body := buildAuthenticatorBody(authn)
	destDir := filepath.Join(cfg.WorkDir, "api")
	if err := core.WriteJSON(destDir, "01-create-authenticator.json", body); err != nil {
		return body, fmt.Errorf("writing authenticator artifact: %w", err)
	}
	return body, nil
}
