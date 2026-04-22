package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RollbackConfig struct {
	WorkDir string
	Plan    *Plan
	Client  APIClient
	DryRun  bool
	Confirm bool
	Verbose bool
}

type RollbackResult struct {
	OperationsRun int
	Skipped       int
}

type RollbackLogEntry struct {
	Timestamp   string `json:"timestamp"`
	OperationID string `json:"operation_id"`
	SourceID    string `json:"source_id,omitempty"`
	Method      string `json:"method,omitempty"`
	Path        string `json:"path,omitempty"`
	Status      int    `json:"status,omitempty"`
	Response    string `json:"response,omitempty"`
	Skipped     bool   `json:"skipped,omitempty"`
	Reason      string `json:"reason,omitempty"`
	DryRun      bool   `json:"dry_run,omitempty"`
}

func Rollback(ctx context.Context, cfg RollbackConfig) (*RollbackResult, error) {
	if cfg.Plan == nil {
		return nil, fmt.Errorf("plan is required")
	}
	if !cfg.Confirm && !cfg.DryRun {
		return nil, fmt.Errorf("rollback requires --confirm")
	}
	if cfg.Client == nil && !cfg.DryRun {
		return nil, fmt.Errorf("client is required unless --dry-run is set")
	}

	applyLog, err := loadApplyLog(cfg.WorkDir)
	if err != nil {
		return nil, err
	}

	operationsByID := make(map[string]Operation, len(cfg.Plan.Operations))
	for _, op := range cfg.Plan.Operations {
		operationsByID[op.ID] = op
	}

	result := &RollbackResult{}
	rollbackLog := make([]RollbackLogEntry, 0, len(applyLog))

	for i := len(applyLog) - 1; i >= 0; i-- {
		entry := applyLog[i]
		if !entryWasApplied(entry) {
			continue
		}

		sourceOp, ok := operationsByID[entry.OperationID]
		if !ok {
			result.Skipped++
			rollbackLog = append(rollbackLog, skippedRollbackEntry(entry, "source operation not found in plan", cfg.DryRun))
			continue
		}

		inverses, reason, ok := inverseOperations(sourceOp, entry, cfg.Plan)
		if !ok {
			result.Skipped++
			rollbackLog = append(rollbackLog, skippedRollbackEntry(entry, reason, cfg.DryRun))
			continue
		}

		for _, inverse := range inverses {
			logEntry := RollbackLogEntry{
				Timestamp:   time.Now().UTC().Format(time.RFC3339),
				OperationID: inverse.ID,
				SourceID:    entry.OperationID,
				Method:      inverse.Method,
				Path:        inverse.Path,
				DryRun:      cfg.DryRun,
			}

			if cfg.DryRun {
				rollbackLog = append(rollbackLog, logEntry)
				result.OperationsRun++
				continue
			}

			status, response, err := executeOperation(ctx, cfg.Client, inverse, nil)
			logEntry.Status = status
			logEntry.Response = string(response)
			rollbackLog = append(rollbackLog, logEntry)

			if err != nil {
				_ = writeRollbackLog(cfg.WorkDir, rollbackLog)
				return nil, fmt.Errorf("%s: %w", inverse.ID, err)
			}
			if status != 200 && status != 202 && status != 204 && status != 404 {
				_ = writeRollbackLog(cfg.WorkDir, rollbackLog)
				return nil, fmt.Errorf("%s: unexpected HTTP %d: %s", inverse.ID, status, string(response))
			}
			result.OperationsRun++
		}
	}

	if err := writeRollbackLog(cfg.WorkDir, rollbackLog); err != nil {
		return nil, err
	}
	if !cfg.DryRun {
		if err := markApplyLogRolledBack(cfg.WorkDir); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func entryWasApplied(entry ApplyLogEntry) bool {
	if entry.DryRun || entry.NoChange || entry.Status == 0 {
		return false
	}
	return entry.Status >= 200 && entry.Status < 300
}

func inverseOperations(sourceOp Operation, entry ApplyLogEntry, plan *Plan) ([]Operation, string, bool) {
	switch rollbackKind(sourceOp) {
	case "group-member":
		workloadID := sourceOp.Metadata["workload_id"]
		if workloadID == "" {
			return nil, "group member operation missing workload_id metadata", false
		}
		memberKind := sourceOp.Metadata["member_kind"]
		if memberKind == "" {
			memberKind = "workload"
		}
		separator := "?"
		if strings.Contains(sourceOp.Path, "?") {
			separator = "&"
		}
		return []Operation{{
			ID:     "rollback-" + sourceOp.ID,
			Method: "DELETE",
			Path:   sourceOp.Path + separator + "id=" + url.QueryEscape(workloadID) + "&kind=" + url.QueryEscape(memberKind),
		}}, "", true

	case "workload-policy":
		workloadIDs := splitMetadataList(sourceOp.Metadata["workload_ids"])
		if len(workloadIDs) == 0 {
			return nil, "workload policy operation missing workload_ids metadata", false
		}
		ops := make([]Operation, 0, len(workloadIDs))
		for i := len(workloadIDs) - 1; i >= 0; i-- {
			workloadID := workloadIDs[i]
			ops = append(ops, Operation{
				ID:     fmt.Sprintf("rollback-delete-workload-%03d", len(workloadIDs)-i),
				Method: "DELETE",
				Path:   workloadDeletePath(workloadID),
			})
		}
		return ops, "", true

	case "authenticator":
		name := sourceOp.Metadata["authenticator_name"]
		if name == "" {
			name = plan.AuthenticatorName
		}
		if name == "" {
			return nil, "authenticator name not found", false
		}
		return []Operation{{
			ID:     "rollback-create-authenticator",
			Method: "DELETE",
			Path:   "/api/authenticators/" + url.PathEscape(name),
		}}, "", true
	}

	return nil, "operation has no rollback mapping", false
}

func rollbackKind(op Operation) string {
	if op.Metadata != nil {
		if kind := strings.TrimSpace(op.Metadata["rollback_kind"]); kind != "" {
			return kind
		}
	}
	switch {
	case strings.HasPrefix(op.ID, "add-group-member-"):
		return "group-member"
	case op.ID == "load-workload-policy":
		return "workload-policy"
	case op.ID == "create-authenticator":
		return "authenticator"
	default:
		return ""
	}
}

func splitMetadataList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var values []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func workloadDeletePath(workloadID string) string {
	return "/api/workloads/" + url.PathEscape(workloadID)
}

func skippedRollbackEntry(entry ApplyLogEntry, reason string, dryRun bool) RollbackLogEntry {
	return RollbackLogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		SourceID:  entry.OperationID,
		Skipped:   true,
		Reason:    reason,
		DryRun:    dryRun,
	}
}

func loadApplyLog(workDir string) ([]ApplyLogEntry, error) {
	path := filepath.Join(workDir, "apply-log.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var entries []ApplyLogEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return entries, nil
}

func writeRollbackLog(workDir string, entries []RollbackLogEntry) error {
	return WriteJSON(workDir, "rollback-log.json", entries)
}

func markApplyLogRolledBack(workDir string) error {
	from := filepath.Join(workDir, "apply-log.json")
	to := filepath.Join(workDir, "apply-log.rolled-back.json")
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("marking apply log as rolled back: %w", err)
	}
	return nil
}
