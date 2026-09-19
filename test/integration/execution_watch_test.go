package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// runCLI runs one command against the live instance with a caller context.
func runCLI(t *testing.T, ctx context.Context, instance integration.Instance, args ...string) (int, string, string) {
	t.Helper()
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(ctx, args, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if strings.Contains(out.String(), instance.APIKey.Reveal()) || strings.Contains(errOut.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into command output")
	}
	return code, out.String(), errOut.String()
}

// TestExecutionWatchCommand polls the live list twice as NDJSON, then cancels
// the way Ctrl+C would. Read-only: it lists metadata and reads workflow names.
func TestExecutionWatchCommand(t *testing.T) {
	instance := integration.Require(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	code, out, errOut := runCLI(t, ctx, instance, "execution", "watch", "--limit", "3", "--interval", "1s", "--output", "json")
	if code != cli.ExitSuccess {
		t.Fatalf("execution watch exit code = %d, want 0 on cancellation (stderr: %s)", code, errOut)
	}
	if strings.Contains(errOut, "canceled") {
		t.Errorf("stderr = %q, want no cancellation diagnostic", errOut)
	}
	for i, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var rec struct {
			SchemaVersion int    `json:"schemaVersion"`
			Type          string `json:"type"`
			ID            string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil || rec.SchemaVersion != 1 || rec.Type != "execution" || rec.ID == "" {
			t.Errorf("line %d is not a versioned execution record (%v): %s", i, err, line)
		}
	}
}

// TestExecutionTraceCommand traces the newest execution on the instance, once
// as text and once as JSON. It skips when the instance has no executions.
func TestExecutionTraceCommand(t *testing.T) {
	instance := integration.Require(t)
	ctx := context.Background()
	page, err := instance.Client(t).ListExecutions(ctx, n8n.ListExecutionsOptions{ListOptions: n8n.ListOptions{Limit: 1}})
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	if len(page.Data) == 0 {
		t.Skip("the instance has no executions to trace")
	}
	newest := page.Data[0]

	code, out, errOut := runCLI(t, ctx, instance, "execution", "trace", newest.ID)
	if code != cli.ExitSuccess {
		t.Fatalf("execution trace exit code = %d (stderr: %s)", code, errOut)
	}
	for _, want := range []string{"Execution: " + newest.ID, "NODE EXECUTION TRACE:", newest.WorkflowID} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}

	code, out, errOut = runCLI(t, ctx, instance, "execution", "trace", "--workflow-id", newest.WorkflowID, "--output", "json")
	if code != cli.ExitSuccess {
		t.Fatalf("execution trace --workflow-id exit code = %d (stderr: %s)", code, errOut)
	}
	var doc struct {
		SchemaVersion int `json:"schemaVersion"`
		Execution     struct {
			ID         string `json:"id"`
			WorkflowID string `json:"workflowId"`
		} `json:"execution"`
		Nodes []json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout is not a trace document: %v\n%s", err, out)
	}
	if doc.SchemaVersion != 1 || doc.Execution.WorkflowID != newest.WorkflowID || doc.Nodes == nil {
		t.Errorf("document = %+v, want schemaVersion 1, the workflow and a nodes array", doc)
	}
}
