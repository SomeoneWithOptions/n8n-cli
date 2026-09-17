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

// TestListAndGetRoles is read-only. It verifies grouped role decoding,
// licensing, usage counts, and one slug round trip without changing identity
// configuration on the target instance.
func TestListAndGetRoles(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)
	roles, err := client.ListRoles(context.Background(), true)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	all := append(append([]n8n.Role(nil), roles.Global...), roles.Project...)
	for _, role := range all {
		if role.Slug == "" || role.DisplayName == "" || role.Licensed == nil {
			t.Errorf("role is missing slug, display name, or licensing: %+v", role)
		}
	}
	if len(all) == 0 {
		t.Log("instance returned no roles")
		return
	}
	got, err := client.GetRole(context.Background(), all[0].Slug, true)
	if err != nil {
		t.Fatalf("GetRole(%q): %v", all[0].Slug, err)
	}
	if got.Slug != all[0].Slug || got.Licensed == nil {
		t.Errorf("GetRole = %+v, want slug %q and licensing", got, all[0].Slug)
	}
}

// TestRoleListCommand runs read-only listing through the full CLI and verifies
// licensing remains present in machine-readable output.
func TestRoleListCommand(t *testing.T) {
	instance := integration.Require(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"role", "list", "--with-usage-count", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("role list exit code = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}
	var roles n8n.Roles
	if err := json.Unmarshal([]byte(out.String()), &roles); err != nil {
		t.Fatalf("stdout is not grouped roles: %v\n%s", err, out.String())
	}
	for _, role := range append(roles.Global, roles.Project...) {
		if role.Licensed == nil {
			t.Errorf("role %q omitted licensing", role.Slug)
		}
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
