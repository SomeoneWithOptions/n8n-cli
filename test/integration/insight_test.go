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

// TestInsightsSummary verifies the read-only client and CLI operation against
// server-default date and project dimensions. Unit tests cover explicit filter
// encoding so live tests do not depend on a supplied project.
func TestInsightsSummary(t *testing.T) {
	instance := integration.Require(t)
	summary, err := instance.Client(t).GetInsightsSummary(context.Background(), n8n.GetInsightsSummaryOptions{})
	if err != nil {
		t.Fatalf("GetInsightsSummary: %v", err)
	}
	if summary.Total.Unit != n8n.InsightUnitCount || summary.Failed.Unit != n8n.InsightUnitCount ||
		summary.FailureRate.Unit != n8n.InsightUnitRatio || summary.TimeSaved.Unit != n8n.InsightUnitMinute ||
		summary.AverageRunTime.Unit != n8n.InsightUnitMillisecond {
		t.Errorf("insight summary has unexpected units: %+v", summary)
	}

	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"insight", "summary", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("insight summary exit code = %d (stderr: %s)", code, errOut.String())
	}
	var cliSummary n8n.InsightsSummary
	if err := json.Unmarshal([]byte(out.String()), &cliSummary); err != nil {
		t.Fatalf("stdout is not an insights summary: %v\n%s", err, out.String())
	}
	if cliSummary.Total.Unit != n8n.InsightUnitCount {
		t.Errorf("CLI insight summary = %+v", cliSummary)
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
