package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// TestListEvaluationRuns checks both pagination layers against a configured
// workflow without starting or cancelling any run. If a run exists, its
// aggregate result and first page of cases are decoded too.
func TestListEvaluationRuns(t *testing.T) {
	instance := integration.Require(t)
	workflowID := integration.RequireEvaluationWorkflowID(t)
	client := instance.Client(t)
	ctx := context.Background()

	page, err := client.ListEvaluationRuns(ctx, workflowID, n8n.ListEvaluationRunsOptions{
		ListOptions: n8n.ListOptions{Limit: 10},
	})
	if err != nil {
		t.Fatalf("ListEvaluationRuns: %v", err)
	}
	for _, run := range page.Data {
		if run.ID == "" || run.Status == "" {
			t.Errorf("evaluation run is missing identifying metadata: %+v", run)
		}
	}
	if len(page.Data) == 0 {
		t.Log("configured workflow has no evaluation runs")
		return
	}

	run, err := client.GetEvaluationRun(ctx, workflowID, page.Data[0].ID)
	if err != nil {
		t.Fatalf("GetEvaluationRun: %v", err)
	}
	if run.ID != page.Data[0].ID || run.Status == "" {
		t.Errorf("evaluation run = %+v", run)
	}
	cases, err := client.ListEvaluationCases(ctx, workflowID, run.ID, n8n.ListEvaluationCasesOptions{
		ListOptions: n8n.ListOptions{Limit: 10},
	})
	if err != nil {
		t.Fatalf("ListEvaluationCases: %v", err)
	}
	for _, testCase := range cases.Data {
		if testCase.ID == "" || testCase.Status == "" {
			t.Errorf("evaluation test case is missing identifying metadata: %+v", testCase)
		}
	}
}

// TestEvaluationListCommand verifies read-only run listing through the CLI.
func TestEvaluationListCommand(t *testing.T) {
	instance := integration.Require(t)
	workflowID := integration.RequireEvaluationWorkflowID(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"evaluation", "list", workflowID, "--limit", "10", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("evaluation list exit code = %d (stderr: %s)", code, errOut.String())
	}
	var page n8n.Page[n8n.EvaluationRunSummary]
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not an evaluation run page: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
