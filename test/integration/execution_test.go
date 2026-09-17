package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// TestListExecutions is read-only and verifies the live page shape. Detailed
// data stays off so the test never downloads an unbounded execution payload.
func TestListExecutions(t *testing.T) {
	instance := integration.Require(t)
	page, err := instance.Client(t).ListExecutions(context.Background(), n8n.ListExecutionsOptions{
		ListOptions: n8n.ListOptions{Limit: 10},
	})
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	for _, execution := range page.Data {
		if execution.ID == "" || execution.WorkflowID == "" || execution.Status == "" {
			t.Errorf("execution is missing identifying metadata: %+v", execution)
		}
	}
	t.Logf("instance returned %d execution(s)", len(page.Data))
}

// TestExecutionListCommand runs read-only metadata listing through the CLI.
func TestExecutionListCommand(t *testing.T) {
	instance := integration.Require(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"execution", "list", "--limit", "10", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("execution list exit code = %d (stderr: %s)", code, errOut.String())
	}
	var page n8n.Page[n8n.Execution]
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not an execution page: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}

// TestExecutionLifecycle creates a prefixed webhook workflow whose Code node
// fails, invokes it once, and touches only the execution that invocation made.
// It exercises detailed read, retry, annotation tags, bulk stop's filtered
// no-match case, single-stop conflict, and delete.
func TestExecutionLifecycle(t *testing.T) {
	instance := integration.RequireDestructive(t)
	client := instance.Client(t)
	ctx := context.Background()
	stamp := time.Now().UnixNano()
	name := fmt.Sprintf("%sexecution-%d", integration.ResourcePrefix, stamp)
	hookPath := fmt.Sprintf("%sexecution-%d", integration.ResourcePrefix, stamp)

	doc := failingExecutionWorkflow(name, hookPath)
	workflow, err := client.CreateWorkflow(ctx, doc)
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.UnpublishWorkflow(context.Background(), workflow.ID)
		if _, err := client.DeleteWorkflow(context.Background(), workflow.ID); err != nil && !n8n.IsNotFound(err) {
			t.Errorf("cleanup DeleteWorkflow(%q): %v", workflow.ID, err)
		}
	})
	if _, err := client.PublishWorkflow(ctx, workflow.ID, n8n.PublishWorkflowRequest{}); err != nil {
		t.Fatalf("PublishWorkflow: %v", err)
	}

	triggerClient := &http.Client{Timeout: 15 * time.Second}
	response, triggerErr := triggerClient.Get(strings.TrimSuffix(instance.URL, "/") + "/webhook/" + hookPath)
	if response != nil {
		_ = response.Body.Close()
	}
	// A failing workflow may answer 500; transport success is not required. A
	// transport error is logged because polling below is the authoritative check.
	if triggerErr != nil {
		t.Logf("webhook request returned %v; polling for its execution", triggerErr)
	}

	execution := waitForWorkflowExecution(t, client, workflow.ID)
	executionIDs := []string{execution.ID}
	t.Cleanup(func() {
		for _, id := range executionIDs {
			if _, err := client.DeleteExecution(context.Background(), id); err != nil && !n8n.IsNotFound(err) {
				t.Errorf("cleanup DeleteExecution(%q): %v", id, err)
			}
		}
	})

	detailed, err := client.GetExecution(ctx, execution.ID, n8n.ExecutionDataOptions{IncludeData: true})
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if detailed.ID != execution.ID || len(detailed.Data) == 0 {
		t.Errorf("detailed execution = %+v, want the generated execution and its run data", detailed)
	}

	// Bulk stop is constrained to the test workflow. Its only run has already
	// failed, so no unrelated or live execution can match.
	stopped, err := client.StopManyExecutions(ctx, n8n.StopManyExecutionsRequest{
		Status: []string{"running"}, WorkflowID: workflow.ID,
	})
	if err != nil {
		t.Fatalf("StopManyExecutions: %v", err)
	}
	if stopped.Stopped != 0 {
		t.Errorf("StopManyExecutions stopped %d, want no running test execution", stopped.Stopped)
	}
	if _, err := client.StopExecution(ctx, execution.ID); err == nil {
		t.Error("StopExecution on a finished execution unexpectedly succeeded")
	}

	retried, err := client.RetryExecution(ctx, execution.ID, n8n.RetryExecutionOptions{})
	if err != nil {
		t.Fatalf("RetryExecution: %v", err)
	}
	if retried.ID == "" || retried.ID == execution.ID {
		t.Fatalf("retried execution = %+v, want a new ID", retried)
	}
	executionIDs = append(executionIDs, retried.ID)

	tag, err := client.CreateTag(ctx, fmt.Sprintf("%sexec-%d", integration.ResourcePrefix, stamp%100000))
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	t.Cleanup(func() {
		if _, err := client.DeleteTag(context.Background(), tag.ID); err != nil && !n8n.IsNotFound(err) {
			t.Errorf("cleanup DeleteTag(%q): %v", tag.ID, err)
		}
	})
	attached, err := client.SetExecutionTags(ctx, execution.ID, []string{tag.ID})
	if err != nil {
		t.Fatalf("SetExecutionTags: %v", err)
	}
	if len(attached) != 1 || attached[0].ID != tag.ID {
		t.Errorf("attached tags = %+v", attached)
	}
	listed, err := client.GetExecutionTags(ctx, execution.ID)
	if err != nil || len(listed) != 1 {
		t.Errorf("GetExecutionTags = %+v, %v; want one", listed, err)
	}
	if cleared, err := client.SetExecutionTags(ctx, execution.ID, nil); err != nil || len(cleared) != 0 {
		t.Errorf("SetExecutionTags(nil) = %+v, %v; want empty", cleared, err)
	}

	deleted, err := client.DeleteExecution(ctx, execution.ID)
	if err != nil {
		t.Fatalf("DeleteExecution: %v", err)
	}
	if deleted.ID.String() != execution.ID {
		t.Errorf("deleted execution = %+v, want %s", deleted, execution.ID)
	}
	executionIDs = executionIDs[1:]
	if _, err := client.GetExecution(ctx, execution.ID, n8n.ExecutionDataOptions{}); !n8n.IsNotFound(err) {
		t.Errorf("GetExecution after delete = %v, want 404", err)
	}
}

func failingExecutionWorkflow(name, hookPath string) n8n.WorkflowDocument {
	nodes := fmt.Sprintf(`[
		{"id":"11111111-1111-4111-8111-111111111111","name":"Webhook","type":"n8n-nodes-base.webhook","typeVersion":2,"position":[0,0],"webhookId":"22222222-2222-4222-8222-222222222222","parameters":{"path":%q,"httpMethod":"GET"}},
		{"id":"33333333-3333-4333-8333-333333333333","name":"Fail","type":"n8n-nodes-base.code","typeVersion":2,"position":[260,0],"parameters":{"jsCode":"throw new Error('n8n-cli integration failure')"}}
	]`, hookPath)
	doc := n8n.WorkflowDocument{
		"nodes":       json.RawMessage(nodes),
		"connections": json.RawMessage(`{"Webhook":{"main":[[{"node":"Fail","type":"main","index":0}]]}}`),
		"settings":    json.RawMessage(`{"executionOrder":"v1"}`),
	}
	if err := doc.Set("name", name); err != nil {
		panic(err)
	}
	return doc
}

func waitForWorkflowExecution(t *testing.T, client *n8n.Client, workflowID string) n8n.Execution {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		page, err := client.ListExecutions(context.Background(), n8n.ListExecutionsOptions{
			ListOptions: n8n.ListOptions{Limit: 10}, WorkflowID: workflowID,
		})
		if err != nil {
			t.Fatalf("ListExecutions while polling: %v", err)
		}
		if len(page.Data) > 0 && page.Data[0].Status != "new" && page.Data[0].Status != "running" {
			return page.Data[0]
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("no completed execution appeared for workflow %q", workflowID)
	return n8n.Execution{}
}
