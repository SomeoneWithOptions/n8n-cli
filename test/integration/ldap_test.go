package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// TestLDAPReads verifies configuration and cursor-paginated history through
// both client and CLI. Configuration PUT is intentionally never exercised on a
// shared instance: loginEnabled=false deletes LDAP identities, and any PUT is
// instance-wide. A dry sync writes a history row, so it has a separate opt-in.
func TestLDAPReads(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)
	configuration, err := client.GetLDAPConfiguration(context.Background())
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) || n8n.IsStatus(err, http.StatusServiceUnavailable) {
		t.Skip("integration instance or credential has no licensed LDAP configuration access")
	}
	if err != nil {
		t.Fatalf("GetLDAPConfiguration: %v", err)
	}
	if configuration.BindingAdminPassword != "" && configuration.BindingAdminPassword != n8n.CredentialBlankingValue {
		t.Error("LDAP client exposed a bind password instead of the blanking placeholder")
	}

	page, err := client.ListLDAPSyncHistory(context.Background(), n8n.ListLDAPSyncHistoryOptions{ListOptions: n8n.ListOptions{Limit: 1}})
	if n8n.IsForbidden(err) {
		t.Skip("integration credential lacks ldap:sync")
	}
	if err != nil {
		t.Fatalf("ListLDAPSyncHistory: %v", err)
	}
	if len(page.Data) > 1 {
		t.Fatalf("history returned %d records for limit=1", len(page.Data))
	}

	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	for _, args := range [][]string{
		{"ldap", "get", "--output", "json"},
		{"ldap", "sync", "history", "--limit", "1", "--output", "json"},
	} {
		var out, errOut strings.Builder
		code := cli.Run(context.Background(), args, cli.Options{
			Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
			Version:     version.Get(),
			ConfigDir:   t.TempDir(),
			Env:         func(name string) string { return env[name] },
			Keyring:     config.NewMemoryStore(config.StorageKeyring),
			Interactive: &interactive,
		})
		if code != cli.ExitSuccess {
			t.Fatalf("%s exit code = %d (stderr: %s)", strings.Join(args, " "), code, errOut.String())
		}
		if !json.Valid([]byte(out.String())) {
			t.Fatalf("%s stdout is not JSON: %s", strings.Join(args, " "), out.String())
		}
		if strings.Contains(out.String(), instance.APIKey.Reveal()) {
			t.Error("API credential leaked into stdout")
		}
	}
}

func TestLDAPDrySync(t *testing.T) {
	instance := integration.Require(t)
	if os.Getenv("N8N_INTEGRATION_LDAP_SYNC") != "1" {
		t.Skip("set N8N_INTEGRATION_LDAP_SYNC=1 to create one dry LDAP sync history record")
	}
	history, err := instance.Client(t).RunLDAPSync(context.Background(), n8n.LDAPSyncRequest{Type: n8n.LDAPSyncDry})
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) || n8n.IsStatus(err, http.StatusServiceUnavailable) {
		t.Skip("integration instance or credential has no licensed LDAP sync access")
	}
	if err != nil {
		t.Fatalf("RunLDAPSync(dry): %v", err)
	}
	if history.RunMode != n8n.LDAPSyncDry {
		t.Errorf("run mode = %q, want dry", history.RunMode)
	}
}
