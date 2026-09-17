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

// TestListAndGetUsers is read-only. It verifies cursor-page decoding, role
// inclusion, and an ID round trip without changing identity configuration.
func TestListAndGetUsers(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)
	page, err := client.ListUsers(context.Background(), n8n.ListUsersOptions{
		ListOptions: n8n.ListOptions{Limit: 10}, IncludeRole: true,
	})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	for _, user := range page.Data {
		if user.ID == "" || user.Email == "" || user.Role == "" {
			t.Errorf("user is missing ID, email, or requested role: %+v", user)
		}
	}
	if len(page.Data) == 0 {
		t.Log("instance returned no users")
		return
	}
	got, err := client.GetUser(context.Background(), page.Data[0].ID, true)
	if err != nil {
		t.Fatalf("GetUser(%q): %v", page.Data[0].ID, err)
	}
	if got.ID != page.Data[0].ID || got.Email == "" || got.Role == "" {
		t.Errorf("GetUser = %+v, want ID %q with email and role", got, page.Data[0].ID)
	}
}

// TestUserListCommand runs read-only listing through the full CLI.
func TestUserListCommand(t *testing.T) {
	instance := integration.Require(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"user", "list", "--limit", "10", "--include-role", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("user list exit code = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}
	var page n8n.Page[n8n.User]
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not a user page: %v\n%s", err, out.String())
	}
	for _, user := range page.Data {
		if user.ID == "" || user.Role == "" {
			t.Errorf("user omitted ID or requested role: %+v", user)
		}
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
