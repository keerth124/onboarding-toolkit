package core

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDryRunChecksBodiesAndWritesLog(t *testing.T) {
	workDir := preparePlanFiles(t)

	result, err := Validate(context.Background(), ValidateConfig{
		WorkDir: workDir,
		Plan:    testPlan(),
		DryRun:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Checked != 2 {
		t.Fatalf("Checked = %d, want 2", result.Checked)
	}

	var log []ValidateLogEntry
	readJSONForCoreTest(t, filepath.Join(workDir, "validate-log.json"), &log)
	if len(log) != 2 {
		t.Fatalf("validate log entries = %d, want 2", len(log))
	}
	for _, entry := range log {
		if entry.Check != "body-readable" || entry.Result != "ok" {
			t.Fatalf("unexpected validate entry: %#v", entry)
		}
	}
}

func TestValidateChecksTenantReachability(t *testing.T) {
	workDir := preparePlanFiles(t)
	client := &fakeAPIClient{
		getResponses: []fakeResponse{
			{status: 200, body: `[]`},
			{status: 404, body: `{"error":"not found"}`},
		},
	}

	result, err := Validate(context.Background(), ValidateConfig{
		WorkDir: workDir,
		Plan:    testPlan(),
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Checked != 2 {
		t.Fatalf("Checked = %d, want 2", result.Checked)
	}
	if len(client.calls) != 2 {
		t.Fatalf("client calls = %d, want 2", len(client.calls))
	}
	if client.calls[0].method != "GET" || client.calls[0].path != "/api/authenticators" {
		t.Fatalf("tenant check call = %#v", client.calls[0])
	}
	if client.calls[1].method != "GET" || client.calls[1].path != "/api/authenticators/github-acme" {
		t.Fatalf("authenticator check call = %#v", client.calls[1])
	}

	var log []ValidateLogEntry
	readJSONForCoreTest(t, filepath.Join(workDir, "validate-log.json"), &log)
	if len(log) != 4 {
		t.Fatalf("validate log entries = %d, want 4", len(log))
	}
	if log[2].OperationID != "tenant-authenticators-list" {
		t.Fatalf("tenant log operation = %q", log[2].OperationID)
	}
	if log[3].OperationID != "authenticator-mode-check" {
		t.Fatalf("authenticator log operation = %q", log[3].OperationID)
	}
}

func TestValidateReportsForbiddenPermissionHint(t *testing.T) {
	workDir := preparePlanFiles(t)
	client := &fakeAPIClient{
		getResponses: []fakeResponse{{status: 403, body: `{"error":"forbidden"}`}},
	}

	_, err := Validate(context.Background(), ValidateConfig{
		WorkDir: workDir,
		Plan:    testPlan(),
		Client:  client,
	})
	if err == nil {
		t.Fatal("expected forbidden error")
	}
	if !strings.Contains(err.Error(), "Authn_Admins") {
		t.Fatalf("err = %q, want Authn_Admins hint", err.Error())
	}

	var log []ValidateLogEntry
	readJSONForCoreTest(t, filepath.Join(workDir, "validate-log.json"), &log)
	if len(log) != 3 {
		t.Fatalf("validate log entries = %d, want 3", len(log))
	}
	if log[2].Status != 403 {
		t.Fatalf("tenant status = %d, want 403", log[2].Status)
	}
}

func TestValidateWarnsOnUnexpectedTenantStatus(t *testing.T) {
	workDir := preparePlanFiles(t)
	client := &fakeAPIClient{
		getResponses: []fakeResponse{
			{status: 404, body: `{"error":"not found"}`},
			{status: 404, body: `{"error":"not found"}`},
		},
	}

	result, err := Validate(context.Background(), ValidateConfig{
		WorkDir: workDir,
		Plan:    testPlan(),
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %#v, want one warning", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "HTTP 404") {
		t.Fatalf("warning = %q, want HTTP 404", result.Warnings[0])
	}
}

func TestValidateWorkloadsOnlyRequiresExistingAuthenticator(t *testing.T) {
	workDir := preparePlanFiles(t)
	plan := testPlan()
	plan.ProvisioningMode = "workloads-only"
	plan.Operations = plan.Operations[1:]
	client := &fakeAPIClient{
		getResponses: []fakeResponse{
			{status: 200, body: `[]`},
			{status: 404, body: `{"error":"not found"}`},
		},
	}

	_, err := Validate(context.Background(), ValidateConfig{
		WorkDir: workDir,
		Plan:    plan,
		Client:  client,
	})
	if err == nil {
		t.Fatal("expected missing authenticator error")
	}
	if !strings.Contains(err.Error(), "run bootstrap first") {
		t.Fatalf("err = %q, want bootstrap hint", err.Error())
	}
}

func TestValidateWorkloadsOnlyAcceptsCompatibleAuthenticator(t *testing.T) {
	workDir := preparePlanFiles(t)
	plan := testPlan()
	plan.ProvisioningMode = "workloads-only"
	plan.Operations = plan.Operations[1:]
	client := &fakeAPIClient{
		getResponses: []fakeResponse{
			{status: 200, body: `[]`},
			{status: 200, body: compatibleAuthenticatorBody()},
		},
	}

	result, err := Validate(context.Background(), ValidateConfig{
		WorkDir: workDir,
		Plan:    plan,
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %#v, want none", result.Warnings)
	}
}

func TestValidateBootstrapFailsOnConflictingAuthenticator(t *testing.T) {
	workDir := preparePlanFiles(t)
	client := &fakeAPIClient{
		getResponses: []fakeResponse{
			{status: 200, body: `[]`},
			{status: 200, body: `{"name":"github-acme","type":"jwt","subtype":"github_actions","data":{"identity":{"identity_path":"data/github-apps/other"}}}`},
		},
	}

	_, err := Validate(context.Background(), ValidateConfig{
		WorkDir: workDir,
		Plan:    testPlan(),
		Client:  client,
	})
	if err == nil {
		t.Fatal("expected conflicting authenticator error")
	}
	if !strings.Contains(err.Error(), "identity_path") {
		t.Fatalf("err = %q, want identity_path conflict", err.Error())
	}
}

func TestValidateFailsOnDeclaredSubtypeConflict(t *testing.T) {
	workDir := preparePlanFiles(t)
	client := &fakeAPIClient{
		getResponses: []fakeResponse{
			{status: 200, body: `[]`},
			{status: 200, body: `{"name":"github-acme","type":"jwt","subtype":"gitlab","data":{"identity":{"identity_path":"data/github-apps/acme"}}}`},
		},
	}

	_, err := Validate(context.Background(), ValidateConfig{
		WorkDir: workDir,
		Plan:    testPlan(),
		Client:  client,
	})
	if err == nil {
		t.Fatal("expected subtype conflict")
	}
	if !strings.Contains(err.Error(), `subtype is "gitlab", want "github_actions"`) {
		t.Fatalf("err = %q, want subtype conflict", err.Error())
	}
}

func TestValidateDoesNotAssumeGitHubSubtypeForOtherPlatforms(t *testing.T) {
	workDir := preparePlanFiles(t)
	plan := testPlan()
	plan.Platform = "gitlab"
	plan.AuthenticatorName = "gitlab-acme"
	plan.AuthenticatorSubtype = ""
	plan.IdentityPath = "data/gitlab-apps/acme"
	client := &fakeAPIClient{
		getResponses: []fakeResponse{
			{status: 200, body: `[]`},
			{status: 200, body: `{"name":"gitlab-acme","type":"jwt","subtype":"gitlab","data":{"identity":{"identity_path":"data/gitlab-apps/acme"}}}`},
		},
	}

	result, err := Validate(context.Background(), ValidateConfig{
		WorkDir: workDir,
		Plan:    plan,
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %#v, want compatible existing authenticator warning", result.Warnings)
	}
}

func TestValidateBootstrapWarnsOnCompatibleExistingAuthenticator(t *testing.T) {
	workDir := preparePlanFiles(t)
	client := &fakeAPIClient{
		getResponses: []fakeResponse{
			{status: 200, body: `[]`},
			{status: 200, body: compatibleAuthenticatorBody()},
		},
	}

	result, err := Validate(context.Background(), ValidateConfig{
		WorkDir: workDir,
		Plan:    testPlan(),
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %#v, want compatible existing authenticator warning", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "already exists") {
		t.Fatalf("warning = %q, want already exists", result.Warnings[0])
	}
}

func TestValidateFailsOnUnreadableBody(t *testing.T) {
	workDir := t.TempDir()
	plan := testPlan()

	_, err := Validate(context.Background(), ValidateConfig{
		WorkDir: workDir,
		Plan:    plan,
		DryRun:  true,
	})
	if err == nil {
		t.Fatal("expected missing body file error")
	}
	if !strings.Contains(err.Error(), "reading body file") {
		t.Fatalf("err = %q, want reading body file", err.Error())
	}
}

func compatibleAuthenticatorBody() string {
	return `{"name":"github-acme","type":"jwt","subtype":"github_actions","data":{"identity":{"identity_path":"data/github-apps/acme"}}}`
}
