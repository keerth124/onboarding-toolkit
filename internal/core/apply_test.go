package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyExecutesPlanAndWritesAuditLog(t *testing.T) {
	workDir := preparePlanFiles(t)
	writeJSONForCoreTest(t, filepath.Join(workDir, "validate-log.json"), []ValidateLogEntry{})
	plan := testPlan()
	client := &fakeAPIClient{
		postResponses: []fakeResponse{
			{status: 201, body: `{"created":"authn"}`},
			{status: 201, body: `{"added":"member"}`},
		},
	}

	result, err := Apply(context.Background(), ApplyConfig{
		WorkDir: workDir,
		Plan:    plan,
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.AuthenticatorName != "github-acme" {
		t.Fatalf("AuthenticatorName = %q, want github-acme", result.AuthenticatorName)
	}
	if result.WorkloadsCreated != 1 {
		t.Fatalf("WorkloadsCreated = %d, want 1", result.WorkloadsCreated)
	}
	if result.MembershipsAdded != 1 {
		t.Fatalf("MembershipsAdded = %d, want 1", result.MembershipsAdded)
	}
	if len(client.calls) != 2 {
		t.Fatalf("client calls = %d, want 2", len(client.calls))
	}
	if client.calls[1].body != "{\"id\":\"data/github-apps/acme/acme/api\",\"kind\":\"workload\"}\n" {
		t.Fatalf("second request body = %q", client.calls[1].body)
	}

	var log []ApplyLogEntry
	readJSONForCoreTest(t, filepath.Join(workDir, "apply-log.json"), &log)
	if len(log) != 2 {
		t.Fatalf("apply log entries = %d, want 2", len(log))
	}
	if log[1].OperationID != "add-group-member-001" {
		t.Fatalf("second log operation = %q", log[1].OperationID)
	}
}

func TestApplyTreatsIdempotentStatusAsNoChange(t *testing.T) {
	workDir := preparePlanFiles(t)
	writeJSONForCoreTest(t, filepath.Join(workDir, "validate-log.json"), []ValidateLogEntry{})
	plan := testPlan()
	client := &fakeAPIClient{
		postResponses: []fakeResponse{
			{status: 409, body: `{"error":"exists"}`},
			{status: 409, body: `{"error":"member exists"}`},
		},
	}

	result, err := Apply(context.Background(), ApplyConfig{
		WorkDir: workDir,
		Plan:    plan,
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.MembershipsAdded != 0 {
		t.Fatalf("MembershipsAdded = %d, want 0 for idempotent no-change", result.MembershipsAdded)
	}

	var log []ApplyLogEntry
	readJSONForCoreTest(t, filepath.Join(workDir, "apply-log.json"), &log)
	if len(log) != 2 {
		t.Fatalf("apply log entries = %d, want 2", len(log))
	}
	for _, entry := range log {
		if !entry.NoChange {
			t.Fatalf("entry %s NoChange = false, want true", entry.OperationID)
		}
	}
}

func TestApplyStopsAtFirstUnexpectedStatusAndWritesPartialLog(t *testing.T) {
	workDir := preparePlanFiles(t)
	writeJSONForCoreTest(t, filepath.Join(workDir, "validate-log.json"), []ValidateLogEntry{})
	plan := testPlan()
	plan.Operations = append(plan.Operations, Operation{
		ID:             "unreached",
		Method:         "POST",
		Path:           "/api/unreached",
		BodyFile:       "api/01-create-authenticator.json",
		ContentType:    "application/json",
		ExpectedStatus: []int{201},
	})
	client := &fakeAPIClient{
		postResponses: []fakeResponse{
			{status: 201, body: `{"created":"authn"}`},
			{status: 500, body: `{"error":"boom"}`},
			{status: 201, body: `{"should":"not run"}`},
		},
	}

	_, err := Apply(context.Background(), ApplyConfig{
		WorkDir: workDir,
		Plan:    plan,
		Client:  client,
	})
	if err == nil {
		t.Fatal("expected unexpected status error")
	}
	if !strings.Contains(err.Error(), "add-group-member-001") {
		t.Fatalf("error = %q, want operation id", err.Error())
	}
	if len(client.calls) != 2 {
		t.Fatalf("client calls = %d, want 2", len(client.calls))
	}

	var log []ApplyLogEntry
	readJSONForCoreTest(t, filepath.Join(workDir, "apply-log.json"), &log)
	if len(log) != 2 {
		t.Fatalf("partial apply log entries = %d, want 2", len(log))
	}
}

func TestApplyRequiresPriorValidateUnlessSkipped(t *testing.T) {
	workDir := preparePlanFiles(t)
	client := &fakeAPIClient{
		postResponses: []fakeResponse{{status: 201}, {status: 201}},
	}

	_, err := Apply(context.Background(), ApplyConfig{
		WorkDir: workDir,
		Plan:    testPlan(),
		Client:  client,
	})
	if err == nil {
		t.Fatal("expected prior validate error")
	}
	if len(client.calls) != 0 {
		t.Fatalf("client calls = %d, want 0", len(client.calls))
	}
}

func TestApplyReturnsClientErrorAndWritesPartialLog(t *testing.T) {
	workDir := preparePlanFiles(t)
	writeJSONForCoreTest(t, filepath.Join(workDir, "validate-log.json"), []ValidateLogEntry{})
	clientErr := errors.New("network down")
	client := &fakeAPIClient{
		postResponses: []fakeResponse{{status: 0, err: clientErr}},
	}

	_, err := Apply(context.Background(), ApplyConfig{
		WorkDir: workDir,
		Plan:    testPlan(),
		Client:  client,
	})
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("err = %v, want network down", err)
	}

	var log []ApplyLogEntry
	readJSONForCoreTest(t, filepath.Join(workDir, "apply-log.json"), &log)
	if len(log) != 1 {
		t.Fatalf("partial apply log entries = %d, want 1", len(log))
	}
}

func preparePlanFiles(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	apiDir := filepath.Join(workDir, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apiDir, "01-create-authenticator.json"), []byte(`{"name":"github-acme"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	members := "{\"id\":\"data/github-apps/acme/acme/api\",\"kind\":\"workload\"}\n"
	if err := os.WriteFile(filepath.Join(apiDir, "03-add-group-members.jsonl"), []byte(members), 0o644); err != nil {
		t.Fatal(err)
	}
	return workDir
}

func testPlan() *Plan {
	return &Plan{
		Version:              "v1alpha1",
		Platform:             "github",
		Tenant:               "myco",
		AuthenticatorType:    "jwt",
		AuthenticatorSubtype: "github_actions",
		AuthenticatorName:    "github-acme",
		ProvisioningMode:     "bootstrap",
		IdentityPath:         "data/github-apps/acme",
		WorkloadCount:        1,
		Operations: []Operation{
			{
				ID:             "create-authenticator",
				Method:         "POST",
				Path:           "/api/authenticators",
				BodyFile:       "api/01-create-authenticator.json",
				ContentType:    "application/json",
				ExpectedStatus: []int{201},
				IdempotentOn:   []int{409},
			},
			{
				ID:             "add-group-member-001",
				Method:         "POST",
				Path:           "/api/groups/conjur%2Fauthn-jwt%2Fgithub-acme%2Fapps/members",
				BodyFile:       "api/03-add-group-members.jsonl",
				BodyLine:       1,
				ContentType:    "application/json",
				ExpectedStatus: []int{201},
				IdempotentOn:   []int{409},
			},
		},
	}
}

type fakeAPIClient struct {
	postResponses   []fakeResponse
	getResponses    []fakeResponse
	deleteResponses []fakeResponse
	calls           []fakeCall
}

type fakeResponse struct {
	status int
	body   string
	err    error
}

type fakeCall struct {
	method      string
	path        string
	contentType string
	body        string
}

func (f *fakeAPIClient) Post(ctx context.Context, path string, contentType string, body []byte) (int, []byte, error) {
	f.calls = append(f.calls, fakeCall{
		method:      "POST",
		path:        path,
		contentType: contentType,
		body:        string(body),
	})
	return f.next(&f.postResponses)
}

func (f *fakeAPIClient) Delete(ctx context.Context, path string) (int, []byte, error) {
	f.calls = append(f.calls, fakeCall{method: "DELETE", path: path})
	return f.next(&f.deleteResponses)
}

func (f *fakeAPIClient) Get(ctx context.Context, path string) (int, []byte, error) {
	f.calls = append(f.calls, fakeCall{method: "GET", path: path})
	return f.next(&f.getResponses)
}

func (f *fakeAPIClient) next(responses *[]fakeResponse) (int, []byte, error) {
	if len(*responses) == 0 {
		return 500, []byte(`{"error":"missing fake response"}`), nil
	}
	response := (*responses)[0]
	*responses = (*responses)[1:]
	return response.status, []byte(response.body), response.err
}

func writeJSONForCoreTest(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSONForCoreTest(t *testing.T, path string, dest any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatal(err)
	}
}
