package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// TestRoleMappingRulesRead is read-only. Creating, moving, or deleting a rule
// changes which roles real users receive at their next sign-in, so no write is
// run against a shared instance; write behavior is covered by the client and
// CLI tests.
func TestRoleMappingRulesRead(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	page, err := client.ListRoleMappingRules(context.Background(), n8n.ListRoleMappingRulesOptions{})
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) || n8n.IsStatus(err, http.StatusServiceUnavailable) {
		t.Skip("integration instance or credential has no role-mapping rule access")
	}
	if err != nil {
		t.Fatalf("ListRoleMappingRules: %v", err)
	}
	for _, rule := range page.Data {
		if rule.ID == "" || rule.Expression == "" || rule.Role == "" {
			t.Errorf("rule is missing ID, expression, or role: %+v", rule)
		}
		if rule.Type != n8n.RoleMappingRuleTypeInstance && rule.Type != n8n.RoleMappingRuleTypeProject {
			t.Errorf("rule %q has undocumented type %q", rule.ID, rule.Type)
		}
		if rule.Order < 0 {
			t.Errorf("rule %q has negative order %d", rule.ID, rule.Order)
		}
	}

	// The type filter returns one evaluation order, so every returned rule
	// must be of that type and no rule may exceed the unfiltered count.
	instanceRules, err := client.ListRoleMappingRules(context.Background(), n8n.ListRoleMappingRulesOptions{
		Type: n8n.RoleMappingRuleTypeInstance,
	})
	if err != nil {
		t.Fatalf("ListRoleMappingRules(instance): %v", err)
	}
	for _, rule := range instanceRules.Data {
		if rule.Type != n8n.RoleMappingRuleTypeInstance {
			t.Errorf("type filter returned a %q rule: %+v", rule.Type, rule)
		}
	}
	if !page.HasMore() && len(instanceRules.Data) > len(page.Data) {
		t.Errorf("filtered rules = %d, want at most the unfiltered %d", len(instanceRules.Data), len(page.Data))
	}
}

// TestRoleMappingListCommand runs read-only listing through the full CLI.
func TestRoleMappingListCommand(t *testing.T) {
	instance := integration.Require(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"role-mapping", "list", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		if strings.Contains(errOut.String(), "403") || strings.Contains(errOut.String(), "404") || strings.Contains(errOut.String(), "503") {
			t.Skip("integration instance or credential has no role-mapping rule access")
		}
		t.Fatalf("role-mapping list exit code = %d (stderr: %s)", code, errOut.String())
	}
	var page n8n.Page[n8n.RoleMappingRule]
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not a rule page: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
