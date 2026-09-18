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

// TestGetSecurityPolicy verifies the read-only operation through both client
// and CLI. PUT is deliberately never exercised on the shared integration
// instance because the policy is instance-wide and cannot be scoped to a
// disposable test resource.
func TestGetSecurityPolicy(t *testing.T) {
	instance := integration.Require(t)
	policy, err := instance.Client(t).GetSecurityPolicy(context.Background())
	if n8n.IsForbidden(err) {
		t.Skip("integration instance lacks securitySettings:manage or the Personal Space Policy license")
	}
	if err != nil {
		t.Fatalf("GetSecurityPolicy: %v", err)
	}
	switch policy.RedactionEnforcement.Floor {
	case n8n.SecurityRedactionOff, n8n.SecurityRedactionProduction, n8n.SecurityRedactionAll:
	default:
		t.Errorf("unexpected redaction floor %q", policy.RedactionEnforcement.Floor)
	}

	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"security-policy", "get", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("security-policy get exit code = %d (stderr: %s)", code, errOut.String())
	}
	var cliPolicy n8n.SecurityPolicy
	if err := json.Unmarshal([]byte(out.String()), &cliPolicy); err != nil {
		t.Fatalf("stdout is not a security policy: %v\n%s", err, out.String())
	}
	if cliPolicy.RedactionEnforcement.Floor != policy.RedactionEnforcement.Floor {
		t.Errorf("CLI floor = %q, client floor = %q", cliPolicy.RedactionEnforcement.Floor, policy.RedactionEnforcement.Floor)
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
