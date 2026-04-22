package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type ValidateConfig struct {
	WorkDir string
	Plan    *Plan
	Client  APIClient
	DryRun  bool
	Verbose bool
}

type ValidateResult struct {
	Checked  int
	Warnings []string
}

type ValidateLogEntry struct {
	Timestamp   string `json:"timestamp"`
	OperationID string `json:"operation_id"`
	Check       string `json:"check"`
	Status      int    `json:"status,omitempty"`
	Result      string `json:"result"`
}

type authenticatorSnapshot struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Subtype      string `json:"subtype"`
	IdentityPath string `json:"identity_path"`
	Data         struct {
		Identity struct {
			IdentityPath string `json:"identity_path"`
		} `json:"identity"`
	} `json:"data"`
}

// Validate performs non-mutating checks that the generated plan can be read and
// that the tenant state matches the generated provisioning mode.
func Validate(ctx context.Context, cfg ValidateConfig) (*ValidateResult, error) {
	if cfg.Plan == nil {
		return nil, fmt.Errorf("plan is required")
	}
	if cfg.Client == nil && !cfg.DryRun {
		return nil, fmt.Errorf("client is required unless --dry-run is set")
	}

	result := &ValidateResult{}
	log := make([]ValidateLogEntry, 0, len(cfg.Plan.Operations))

	for _, op := range cfg.Plan.Operations {
		if _, err := operationBody(cfg.WorkDir, op); err != nil {
			return nil, err
		}
		result.Checked++
		log = append(log, ValidateLogEntry{
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			OperationID: op.ID,
			Check:       "body-readable",
			Result:      "ok",
		})
	}

	if cfg.DryRun {
		if err := WriteJSON(cfg.WorkDir, "validate-log.json", log); err != nil {
			return nil, err
		}
		return result, nil
	}

	status, body, err := cfg.Client.Get(ctx, "/api/authenticators")
	log = append(log, ValidateLogEntry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		OperationID: "tenant-authenticators-list",
		Check:       "tenant-reachable",
		Status:      status,
		Result:      strings.TrimSpace(string(body)),
	})
	if err != nil {
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("Conjur endpoint reachability check failed: %w", err)
	}
	if status == 401 {
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("Conjur authentication failed while validating plan")
	}
	if status == 403 {
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("Conjur identity lacks permission to list authenticators; check Authn_Admins membership")
	}
	if status >= 400 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Conjur endpoint reachability returned HTTP %d; apply may still fail if API path differs", status))
	}

	if cfg.Plan.AuthenticatorName == "" {
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("authenticator name is required for mode-aware validation")
	}

	mode := cfg.Plan.ProvisioningMode
	if mode == "" {
		mode = "bootstrap"
	}
	if mode != "bootstrap" && mode != "workloads-only" {
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("unsupported provisioning mode %q", mode)
	}

	authnPath := "/api/authenticators/" + url.PathEscape(cfg.Plan.AuthenticatorName)
	status, body, err = cfg.Client.Get(ctx, authnPath)
	log = append(log, ValidateLogEntry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		OperationID: "authenticator-mode-check",
		Check:       mode + "-authenticator-state",
		Status:      status,
		Result:      strings.TrimSpace(string(body)),
	})
	if err != nil {
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("authenticator %q validation failed: %w", cfg.Plan.AuthenticatorName, err)
	}
	if status == 401 {
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("Conjur authentication failed while validating authenticator %q", cfg.Plan.AuthenticatorName)
	}
	if status == 403 {
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("Conjur identity lacks permission to inspect authenticator %q; check Authn_Admins membership", cfg.Plan.AuthenticatorName)
	}

	switch {
	case status == 404 && mode == "workloads-only":
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("workloads-only mode requires existing authenticator %q; run bootstrap first or pass --authenticator-name for the existing org authenticator", cfg.Plan.AuthenticatorName)
	case status == 404 && mode == "bootstrap":
		// Expected first-run state.
	case status >= 200 && status < 300:
		if conflict := authenticatorConflict(cfg.Plan, body); conflict != "" {
			_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
			return nil, fmt.Errorf("existing authenticator %q conflicts with generated plan: %s", cfg.Plan.AuthenticatorName, conflict)
		}
		if mode == "bootstrap" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("authenticator %q already exists and appears compatible; apply may treat creation as no change", cfg.Plan.AuthenticatorName))
		}
	case status >= 400:
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("authenticator %q validation returned HTTP %d: %s", cfg.Plan.AuthenticatorName, status, strings.TrimSpace(string(body)))
	}

	if err := WriteJSON(cfg.WorkDir, "validate-log.json", log); err != nil {
		return nil, err
	}
	return result, nil
}

func authenticatorConflict(plan *Plan, body []byte) string {
	if len(strings.TrimSpace(string(body))) == 0 {
		return ""
	}

	var snapshot authenticatorSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return fmt.Sprintf("could not parse existing authenticator response: %v", err)
	}

	if snapshot.Name != "" && snapshot.Name != plan.AuthenticatorName {
		return fmt.Sprintf("name is %q, want %q", snapshot.Name, plan.AuthenticatorName)
	}
	if snapshot.Type != "" && plan.AuthenticatorType != "" && snapshot.Type != plan.AuthenticatorType {
		return fmt.Sprintf("type is %q, want %q", snapshot.Type, plan.AuthenticatorType)
	}
	if snapshot.Subtype != "" && plan.AuthenticatorSubtype != "" && snapshot.Subtype != plan.AuthenticatorSubtype {
		return fmt.Sprintf("subtype is %q, want %q", snapshot.Subtype, plan.AuthenticatorSubtype)
	}

	existingIdentityPath := snapshot.IdentityPath
	if existingIdentityPath == "" {
		existingIdentityPath = snapshot.Data.Identity.IdentityPath
	}
	if existingIdentityPath != "" && plan.IdentityPath != "" && existingIdentityPath != plan.IdentityPath {
		return fmt.Sprintf("identity_path is %q, want %q", existingIdentityPath, plan.IdentityPath)
	}

	return ""
}
