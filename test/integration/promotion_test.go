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

// skipUnlessPromotions skips when the instance does not serve this group. The
// target instance serves GitConnections instead; both share the
// gitConnection:* scope namespace, so the served paths decide, not the scopes.
func skipUnlessPromotions(t *testing.T, err error) {
	t.Helper()
	switch {
	case n8n.IsNotFound(err), n8n.IsStatus(err, http.StatusServiceUnavailable):
		t.Skip("instance does not serve promotions: a GitConnections-generation instance answers here instead")
	case n8n.IsForbidden(err):
		t.Skip("integration credential lacks gitConnection:list")
	}
}

// TestPromotionProviders verifies the read-only provider list through both
// client and CLI. Mutations are never run against the shared instance: provider
// credentials are instance-wide and connections mutate remote Git state.
func TestPromotionProviders(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	page, err := client.ListPromotionProviders(context.Background(), n8n.ListOptions{})
	skipUnlessPromotions(t, err)
	if err != nil {
		t.Fatalf("ListPromotionProviders: %v", err)
	}
	for _, provider := range page.Data {
		if provider.ID == "" || provider.Name == "" || provider.Type == "" {
			t.Errorf("provider is missing id, name, or type: %+v", provider)
		}
		if provider.AuthType != n8n.PromotionAuthTypeSSHKey && provider.AuthType != n8n.PromotionAuthTypeToken {
			t.Errorf("provider %q auth type = %q, want ssh-key or token", provider.ID, provider.AuthType)
		}
	}
	if len(page.Data) > 0 {
		got, err := client.GetPromotionProvider(context.Background(), page.Data[0].ID)
		skipUnlessPromotions(t, err)
		if err != nil {
			t.Fatalf("GetPromotionProvider(%q): %v", page.Data[0].ID, err)
		}
		if got.ID != page.Data[0].ID {
			t.Errorf("get ID = %q, want %q", got.ID, page.Data[0].ID)
		}
	}

	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"promotion", "provider", "list", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		if strings.Contains(errOut.String(), "403") || strings.Contains(errOut.String(), "404") || strings.Contains(errOut.String(), "503") {
			t.Skip("integration instance has no Promotions module")
		}
		t.Fatalf("promotion provider list exit code = %d (stderr: %s)", code, errOut.String())
	}
	var listPage n8n.Page[n8n.PromotionProvider]
	if err := json.Unmarshal([]byte(out.String()), &listPage); err != nil {
		t.Fatalf("stdout is not a promotion provider page: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("API credential leaked into stdout")
	}
}

// TestPromotionConnections verifies the read-only connection list through both
// client and CLI, with the only documented query filter.
func TestPromotionConnections(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	page, err := client.ListPromotionConnections(context.Background(), n8n.ListPromotionConnectionsOptions{})
	skipUnlessPromotions(t, err)
	if err != nil {
		t.Fatalf("ListPromotionConnections: %v", err)
	}
	for _, connection := range page.Data {
		if connection.ID == "" || connection.Name == "" || connection.Scope == "" {
			t.Errorf("connection is missing id, name, or scope: %+v", connection)
		}
	}
	if _, err := client.ListPromotionConnections(context.Background(), n8n.ListPromotionConnectionsOptions{Scope: n8n.PromotionScopeProjects}); err != nil {
		skipUnlessPromotions(t, err)
		t.Fatalf("ListPromotionConnections with scope: %v", err)
	}
	if len(page.Data) > 0 {
		got, err := client.GetPromotionConnection(context.Background(), page.Data[0].ID)
		skipUnlessPromotions(t, err)
		if err != nil {
			t.Fatalf("GetPromotionConnection(%q): %v", page.Data[0].ID, err)
		}
		if got.ID != page.Data[0].ID {
			t.Errorf("get ID = %q, want %q", got.ID, page.Data[0].ID)
		}
		projects, err := client.ListPromotionConnectionProjects(context.Background(), page.Data[0].ID)
		skipUnlessPromotions(t, err)
		if err != nil {
			t.Fatalf("ListPromotionConnectionProjects(%q): %v", page.Data[0].ID, err)
		}
		_ = projects
	}

	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"promotion", "connection", "list", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		if strings.Contains(errOut.String(), "403") || strings.Contains(errOut.String(), "404") || strings.Contains(errOut.String(), "503") {
			t.Skip("integration instance has no Promotions module")
		}
		t.Fatalf("promotion connection list exit code = %d (stderr: %s)", code, errOut.String())
	}
	var listPage n8n.Page[n8n.PromotionConnection]
	if err := json.Unmarshal([]byte(out.String()), &listPage); err != nil {
		t.Fatalf("stdout is not a promotion connection page: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("API credential leaked into stdout")
	}
}
