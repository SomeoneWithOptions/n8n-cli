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

// TestOIDCRead verifies the licensed configuration endpoint through both client
// and CLI. PUT is intentionally not exercised against a shared instance because
// it changes the instance-wide login flow and may be environment-managed.
func TestOIDCRead(t *testing.T) {
	instance := integration.Require(t)
	configuration, err := instance.Client(t).GetOIDCConfiguration(context.Background())
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) || n8n.IsStatus(err, http.StatusServiceUnavailable) {
		t.Skip("integration instance or credential has no licensed OIDC configuration access")
	}
	if err != nil {
		t.Fatalf("GetOIDCConfiguration: %v", err)
	}
	if configuration.ClientSecret != "" && configuration.ClientSecret != n8n.OIDCClientSecretRedactedValue {
		t.Error("OIDC client exposed a client secret instead of the redacted placeholder")
	}

	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"oidc", "get", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("oidc get exit code = %d (stderr: %s)", code, errOut.String())
	}
	if !json.Valid([]byte(out.String())) {
		t.Fatalf("oidc get stdout is not JSON: %s", out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("API credential leaked into stdout")
	}
	var cliConfiguration n8n.OIDCConfiguration
	if err := json.Unmarshal([]byte(out.String()), &cliConfiguration); err != nil {
		t.Fatalf("decode oidc get output: %v", err)
	}
	if cliConfiguration.ClientSecret != "" && cliConfiguration.ClientSecret != n8n.OIDCClientSecretRedactedValue {
		t.Error("OIDC CLI exposed a client secret instead of the redacted placeholder")
	}
}
