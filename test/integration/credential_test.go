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

// TestListCredentials is read-only. The integration API key must belong to an
// instance owner or admin because n8n restricts this endpoint by role.
func TestListCredentials(t *testing.T) {
	instance := integration.Require(t)
	page, err := instance.Client(t).ListCredentials(context.Background(), n8n.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	for _, credential := range page.Data {
		if credential.ID == "" || credential.Name == "" || credential.Type == "" {
			t.Errorf("credential is missing metadata: %+v", credential)
		}
		encoded, err := json.Marshal(credential)
		if err != nil {
			t.Fatalf("marshal credential metadata: %v", err)
		}
		if strings.Contains(string(encoded), `"data"`) {
			t.Errorf("credential output unexpectedly contains data: %s", encoded)
		}
	}
	t.Logf("instance returned %d credential metadata record(s)", len(page.Data))
}

// TestCredentialSchema checks the no-additional-scope schema endpoint using a
// built-in credential type present on standard n8n installations.
func TestCredentialSchema(t *testing.T) {
	instance := integration.Require(t)
	schema, err := instance.Client(t).CredentialSchema(context.Background(), "httpBasicAuth")
	if err != nil {
		t.Fatalf("CredentialSchema: %v", err)
	}
	if !json.Valid(schema) || !strings.Contains(string(schema), `"properties"`) {
		t.Errorf("schema is not an object with properties: %s", schema)
	}
}

// TestCredentialListCommand verifies metadata-only JSON through the full CLI.
func TestCredentialListCommand(t *testing.T) {
	instance := integration.Require(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"credential", "list", "--limit", "10", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("credential list exit = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}
	var page n8n.Page[n8n.Credential]
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not credential page JSON: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) || strings.Contains(out.String(), `"data":{"`) {
		t.Error("credential or API key leaked into stdout")
	}
}
