package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRollbackRequiresConfirm(t *testing.T) {
	workDir := prepareRollbackFiles(t)
	client := &fakeAPIClient{}

	_, err := Rollback(context.Background(), RollbackConfig{
		WorkDir: workDir,
		Plan:    testRollbackPlan(),
		Client:  client,
	})
	if err == nil {
		t.Fatal("expected confirm error")
	}
	if len(client.calls) != 0 {
		t.Fatalf("client calls = %d, want 0", len(client.calls))
	}
}

func TestRollbackRunsInverseOperationsInReverseOrder(t *testing.T) {
	workDir := prepareRollbackFiles(t)
	client := &fakeAPIClient{
		deleteResponses: []fakeResponse{
			{status: 204, body: ""},
			{status: 204, body: ""},
			{status: 204, body: ""},
		},
	}

	result, err := Rollback(context.Background(), RollbackConfig{
		WorkDir: workDir,
		Plan:    testRollbackPlan(),
		Client:  client,
		Confirm: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.OperationsRun != 3 {
		t.Fatalf("OperationsRun = %d, want 3", result.OperationsRun)
	}
	if result.Skipped != 0 {
		t.Fatalf("Skipped = %d, want 0", result.Skipped)
	}
	if len(client.calls) != 3 {
		t.Fatalf("client calls = %d, want 3", len(client.calls))
	}
	if client.calls[0].method != "DELETE" || !strings.HasPrefix(client.calls[0].path, "/api/groups/") {
		t.Fatalf("first call = %#v, want group membership delete", client.calls[0])
	}
	if client.calls[1].method != "DELETE" || client.calls[1].path != "/api/workloads/data%2Fgithub-apps%2Facme%2Facme%2Fapi" {
		t.Fatalf("second call = %#v, want workload delete", client.calls[1])
	}
	if client.calls[2].method != "DELETE" || client.calls[2].path != "/api/authenticators/github-acme" {
		t.Fatalf("third call = %#v, want authenticator delete", client.calls[2])
	}

	var log []RollbackLogEntry
	readJSONForCoreTest(t, filepath.Join(workDir, "rollback-log.json"), &log)
	if len(log) != 3 {
		t.Fatalf("rollback log entries = %d, want 3", len(log))
	}
	assertFileExists(t, filepath.Join(workDir, "apply-log.rolled-back.json"))
	assertFileMissing(t, filepath.Join(workDir, "apply-log.json"))
}

func TestRollbackTreatsNotFoundAsSuccess(t *testing.T) {
	workDir := prepareRollbackFiles(t)
	client := &fakeAPIClient{
		deleteResponses: []fakeResponse{
			{status: 404, body: `{"error":"not found"}`},
			{status: 404, body: `{"error":"not found"}`},
			{status: 404, body: `{"error":"not found"}`},
		},
	}

	_, err := Rollback(context.Background(), RollbackConfig{
		WorkDir: workDir,
		Plan:    testRollbackPlan(),
		Client:  client,
		Confirm: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRollbackDryRunDoesNotMoveApplyLog(t *testing.T) {
	workDir := prepareRollbackFiles(t)

	result, err := Rollback(context.Background(), RollbackConfig{
		WorkDir: workDir,
		Plan:    testRollbackPlan(),
		DryRun:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationsRun != 3 {
		t.Fatalf("OperationsRun = %d, want 3", result.OperationsRun)
	}
	assertFileExists(t, filepath.Join(workDir, "apply-log.json"))
	assertFileMissing(t, filepath.Join(workDir, "apply-log.rolled-back.json"))
}

func TestRollbackStopsOnUnexpectedStatus(t *testing.T) {
	workDir := prepareRollbackFiles(t)
	client := &fakeAPIClient{
		deleteResponses: []fakeResponse{
			{status: 500, body: `{"error":"boom"}`},
		},
	}

	_, err := Rollback(context.Background(), RollbackConfig{
		WorkDir: workDir,
		Plan:    testRollbackPlan(),
		Client:  client,
		Confirm: true,
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}
	if !strings.Contains(err.Error(), "unexpected HTTP 500") {
		t.Fatalf("err = %q, want HTTP 500", err.Error())
	}
	assertFileExists(t, filepath.Join(workDir, "apply-log.json"))
}

func prepareRollbackFiles(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	entries := []ApplyLogEntry{
		{
			OperationID: "create-authenticator",
			Method:      "POST",
			Path:        "/api/authenticators",
			Status:      201,
		},
		{
			OperationID: "load-workload-policy",
			Method:      "POST",
			Path:        "/policies/conjur/policy/root",
			Status:      201,
		},
		{
			OperationID: "add-group-member-001",
			Method:      "POST",
			Path:        "/api/groups/conjur%2Fauthn-jwt%2Fgithub-acme%2Fapps/members",
			Status:      201,
		},
	}
	writeJSONForCoreTest(t, filepath.Join(workDir, "apply-log.json"), entries)
	return workDir
}

func testRollbackPlan() *Plan {
	plan := testPlan()
	plan.Operations[0].IdempotentOn = nil
	plan.Operations[1].Metadata = map[string]string{
		"workload_id": "data/github-apps/acme/acme/api",
		"group_id":    "conjur%2Fauthn-jwt%2Fgithub-acme%2Fapps",
	}
	plan.Operations = append(plan.Operations[:1], append([]Operation{
		{
			ID:             "load-workload-policy",
			Method:         "POST",
			Path:           "/policies/conjur/policy/root",
			BodyFile:       "api/02-workloads.yml",
			ExpectedStatus: []int{201},
			Metadata: map[string]string{
				"workload_ids": "data/github-apps/acme/acme/api",
			},
		},
	}, plan.Operations[1:]...)...)
	return plan
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to be missing", path)
	}
}
