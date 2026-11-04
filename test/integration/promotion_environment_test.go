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

// TestPromotionEnvironments verifies the read-only environment list, get, and
// project list through both client and CLI. Instances older than n8n 2.46
// serve connections instead and answer 404, which skips. Nothing is created,
// connected, or linked: environments mutate instance-wide sync state.
func TestPromotionEnvironments(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)
	ctx := context.Background()

	page, err := client.ListPromotionEnvironments(ctx, n8n.ListPromotionEnvironmentsOptions{})
	skipUnlessPromotions(t, err)
	if err != nil {
		t.Fatalf("ListPromotionEnvironments: %v", err)
	}
	for _, environment := range page.Data {
		if environment.ID == "" || environment.Name == "" || environment.Scope == "" {
			t.Errorf("environment is missing id, name, or scope: %+v", environment)
		}
		if environment.Direction != n8n.PromotionDirectionApply && environment.Direction != n8n.PromotionDirectionPromote {
			t.Errorf("environment %q direction = %q, want apply or promote", environment.ID, environment.Direction)
		}
		if environment.Settings.Direction() != environment.Direction {
			t.Errorf("environment %q settings %+v do not match direction %q", environment.ID, environment.Settings, environment.Direction)
		}
	}
	filtered, err := client.ListPromotionEnvironments(ctx, n8n.ListPromotionEnvironmentsOptions{Direction: n8n.PromotionDirectionApply})
	skipUnlessPromotions(t, err)
	if err != nil {
		t.Fatalf("ListPromotionEnvironments with direction: %v", err)
	}
	for _, environment := range filtered.Data {
		if environment.Direction != n8n.PromotionDirectionApply {
			t.Errorf("direction filter returned %q environment %q", environment.Direction, environment.ID)
		}
	}
	if len(page.Data) > 0 {
		first := page.Data[0]
		got, err := client.GetPromotionEnvironment(ctx, first.ID)
		skipUnlessPromotions(t, err)
		if err != nil {
			t.Fatalf("GetPromotionEnvironment(%q): %v", first.ID, err)
		}
		if got.ID != first.ID || got.Checkout == nil {
			t.Errorf("get = %+v, want ID %q with a checkout state", got, first.ID)
		}
		if _, err := client.ListPromotionEnvironmentProjects(ctx, first.ID); err != nil {
			skipUnlessPromotions(t, err)
			t.Fatalf("ListPromotionEnvironmentProjects(%q): %v", first.ID, err)
		}
	}

	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(ctx, []string{"promotion", "environment", "list", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		if strings.Contains(errOut.String(), "403") || strings.Contains(errOut.String(), "404") || strings.Contains(errOut.String(), "503") {
			t.Skip("integration instance has no promotion environments")
		}
		t.Fatalf("promotion environment list exit code = %d (stderr: %s)", code, errOut.String())
	}
	var listPage n8n.Page[n8n.PromotionEnvironment]
	if err := json.Unmarshal([]byte(out.String()), &listPage); err != nil {
		t.Fatalf("stdout is not a promotion environment page: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("API credential leaked into stdout")
	}
}
