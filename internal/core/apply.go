package core

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// APIClient is the subset of the Conjur client used by the provisioning core.
type APIClient interface {
	Post(ctx context.Context, path string, contentType string, body []byte) (int, []byte, error)
	Delete(ctx context.Context, path string) (int, []byte, error)
	Get(ctx context.Context, path string) (int, []byte, error)
}

type ApplyConfig struct {
	WorkDir      string
	Plan         *Plan
	Client       APIClient
	DryRun       bool
	Verbose      bool
	SkipValidate bool
}

type ApplyResult struct {
	AuthenticatorName string
	WorkloadsCreated  int
	MembershipsAdded  int
}

type ApplyLogEntry struct {
	Timestamp   string `json:"timestamp"`
	OperationID string `json:"operation_id"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	RequestBody string `json:"request_body,omitempty"`
	Status      int    `json:"status"`
	Response    string `json:"response,omitempty"`
	NoChange    bool   `json:"no_change,omitempty"`
	DryRun      bool   `json:"dry_run,omitempty"`
}

// Apply executes a generated API plan in order and records an audit log.
func Apply(ctx context.Context, cfg ApplyConfig) (*ApplyResult, error) {
	if cfg.Plan == nil {
		return nil, fmt.Errorf("plan is required")
	}
	if cfg.Client == nil && !cfg.DryRun {
		return nil, fmt.Errorf("client is required unless --dry-run is set")
	}
	if !cfg.SkipValidate && !cfg.DryRun {
		validateLog := filepath.Join(cfg.WorkDir, "validate-log.json")
		if _, err := os.Stat(validateLog); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("prior validate is required; run validate first or pass --skip-validate")
			}
			return nil, fmt.Errorf("checking validate log: %w", err)
		}
	}

	log := make([]ApplyLogEntry, 0, len(cfg.Plan.Operations))
	result := &ApplyResult{
		AuthenticatorName: cfg.Plan.AuthenticatorName,
		WorkloadsCreated:  cfg.Plan.WorkloadCount,
	}

	for _, op := range cfg.Plan.Operations {
		body, err := operationBody(cfg.WorkDir, op)
		if err != nil {
			return nil, err
		}

		entry := ApplyLogEntry{
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			OperationID: op.ID,
			Method:      op.Method,
			Path:        op.Path,
			RequestBody: string(body),
			DryRun:      cfg.DryRun,
		}

		if cfg.DryRun {
			log = append(log, entry)
			if strings.HasPrefix(op.ID, "add-group-member-") {
				result.MembershipsAdded++
			}
			continue
		}

		status, response, err := executeOperation(ctx, cfg.Client, op, body)
		entry.Status = status
		entry.Response = string(response)
		entry.NoChange = containsStatus(op.IdempotentOn, status)
		log = append(log, entry)

		if err != nil {
			_ = writeApplyLog(cfg.WorkDir, log)
			return nil, fmt.Errorf("%s: %w", op.ID, err)
		}
		if !containsStatus(op.ExpectedStatus, status) && !containsStatus(op.IdempotentOn, status) {
			_ = writeApplyLog(cfg.WorkDir, log)
			return nil, fmt.Errorf("%s: unexpected HTTP %d: %s", op.ID, status, string(response))
		}

		if strings.HasPrefix(op.ID, "add-group-member-") && !entry.NoChange {
			result.MembershipsAdded++
		}
	}

	if err := writeApplyLog(cfg.WorkDir, log); err != nil {
		return nil, err
	}
	return result, nil
}

func executeOperation(ctx context.Context, client APIClient, op Operation, body []byte) (int, []byte, error) {
	switch op.Method {
	case "POST":
		return client.Post(ctx, op.Path, op.ContentType, body)
	case "DELETE":
		return client.Delete(ctx, op.Path)
	case "GET":
		return client.Get(ctx, op.Path)
	default:
		return 0, nil, fmt.Errorf("unsupported method %q", op.Method)
	}
}

func writeApplyLog(workDir string, entries []ApplyLogEntry) error {
	return WriteJSON(workDir, "apply-log.json", entries)
}

func operationBody(workDir string, op Operation) ([]byte, error) {
	if op.BodyFile == "" {
		return nil, nil
	}

	path := filepath.Join(workDir, filepath.FromSlash(op.BodyFile))
	if op.BodyLine <= 0 {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: reading body file %s: %w", op.ID, path, err)
		}
		return data, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s: opening body file %s: %w", op.ID, path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if lineNo == op.BodyLine {
			return append([]byte(scanner.Text()), '\n'), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: scanning %s: %w", op.ID, path, err)
	}
	return nil, fmt.Errorf("%s: body line %d not found in %s", op.ID, op.BodyLine, path)
}
