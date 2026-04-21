package core

import (
	"context"
	"fmt"
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

// Validate performs non-mutating checks that the generated plan can be read and
// that the tenant is reachable. It intentionally avoids pretending to verify API
// endpoints whose exact shape is still called out as an open PRD question.
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
		return nil, fmt.Errorf("tenant reachability check failed: %w", err)
	}
	if status == 401 {
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("tenant authentication failed while validating plan")
	}
	if status == 403 {
		_ = WriteJSON(cfg.WorkDir, "validate-log.json", log)
		return nil, fmt.Errorf("tenant identity lacks permission to list authenticators; check Authn_Admins membership")
	}
	if status >= 400 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("tenant reachability returned HTTP %d; apply may still fail if API path differs", status))
	}

	if err := WriteJSON(cfg.WorkDir, "validate-log.json", log); err != nil {
		return nil, err
	}
	return result, nil
}
