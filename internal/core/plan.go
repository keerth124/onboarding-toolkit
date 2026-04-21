package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Operation describes one generated API call in execution order.
type Operation struct {
	ID             string            `json:"id"`
	Description    string            `json:"description"`
	Method         string            `json:"method"`
	Path           string            `json:"path"`
	BodyFile       string            `json:"body_file,omitempty"`
	BodyLine       int               `json:"body_line,omitempty"`
	ContentType    string            `json:"content_type,omitempty"`
	ExpectedStatus []int             `json:"expected_status"`
	IdempotentOn   []int             `json:"idempotent_on,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// Plan is the manifest consumed by validate, apply, and rollback.
type Plan struct {
	Version           string      `json:"version"`
	Platform          string      `json:"platform"`
	Tenant            string      `json:"tenant"`
	ConjurURL         string      `json:"conjur_url,omitempty"`
	ConjurTarget      string      `json:"conjur_target,omitempty"`
	AuthenticatorType string      `json:"authenticator_type"`
	AuthenticatorName string      `json:"authenticator_name"`
	ProvisioningMode  string      `json:"provisioning_mode,omitempty"`
	AppsGroupID       string      `json:"apps_group_id"`
	IdentityPath      string      `json:"identity_path"`
	WorkloadCount     int         `json:"workload_count"`
	Operations        []Operation `json:"operations"`
}

// LoadPlan reads api/plan.json from a working directory.
func LoadPlan(workDir string) (*Plan, error) {
	path := filepath.Join(workDir, "api", "plan.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &plan, nil
}

func containsStatus(statuses []int, status int) bool {
	for _, s := range statuses {
		if s == status {
			return true
		}
	}
	return false
}
