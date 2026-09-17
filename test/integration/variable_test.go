package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// TestListVariables is read-only. Values are part of the documented response,
// but the test never logs them.
func TestListVariables(t *testing.T) {
	instance := integration.Require(t)
	page, err := instance.Client(t).ListVariables(context.Background(), n8n.ListVariablesOptions{
		ListOptions: n8n.ListOptions{Limit: 10},
	})
	if err != nil {
		t.Fatalf("ListVariables: %v", err)
	}
	for _, variable := range page.Data {
		if variable.ID == "" || variable.Key == "" {
			t.Errorf("variable is missing its ID or key")
		}
	}
	t.Logf("instance returned %d variable(s), next cursor %q", len(page.Data), page.NextCursor)
}

// TestVariableLifecycle creates one global variable, discovers its server ID,
// replaces it and deletes it. It refuses to touch a pre-existing key.
func TestVariableLifecycle(t *testing.T) {
	instance := integration.RequireDestructive(t)
	client := instance.Client(t)
	ctx := context.Background()
	key := fmt.Sprintf("n8n_cli_test_%08d", time.Now().UnixNano()%1e8)
	value := fmt.Sprintf("integration-%d", time.Now().UnixNano())

	existing, err := collectVariables(ctx, client)
	if err != nil {
		t.Fatalf("preflight ListVariables: %v", err)
	}
	for _, variable := range existing {
		if variable.Key == key && variable.Project == nil {
			t.Fatalf("refusing to touch pre-existing global variable %q", key)
		}
	}

	if err := client.CreateVariable(ctx, n8n.VariableRequest{Key: key, Value: value}); err != nil {
		t.Fatalf("CreateVariable(%q): %v", key, err)
	}
	var created n8n.Variable
	deleted := false
	t.Cleanup(func() {
		if deleted || created.ID == "" {
			return
		}
		if err := client.DeleteVariable(context.Background(), created.ID); err != nil {
			t.Errorf("cleanup DeleteVariable(%q): %v", created.ID, err)
		}
	})

	variables, err := collectVariables(ctx, client)
	if err != nil {
		t.Fatalf("ListVariables after create: %v", err)
	}
	for _, variable := range variables {
		if variable.Key == key && variable.Project == nil {
			created = variable
			break
		}
	}
	if created.ID == "" || created.Value != value {
		t.Fatal("created global variable was not returned with its ID and expected value")
	}

	if err := client.CreateVariable(ctx, n8n.VariableRequest{Key: key, Value: "duplicate"}); !n8n.IsConflict(err) {
		t.Errorf("duplicate CreateVariable error = %v, want 409", err)
	}

	updatedKey := key + "_updated"
	updatedValue := value + "-updated"
	if err := client.UpdateVariable(ctx, created.ID, n8n.VariableRequest{Key: updatedKey, Value: updatedValue}); err != nil {
		t.Fatalf("UpdateVariable(%q): %v", created.ID, err)
	}
	variables, err = collectVariables(ctx, client)
	if err != nil {
		t.Fatalf("ListVariables after update: %v", err)
	}
	found := false
	for _, variable := range variables {
		if variable.ID == created.ID {
			found = variable.Key == updatedKey && variable.Value == updatedValue && variable.Project == nil
		}
	}
	if !found {
		t.Error("updated global variable was not returned with its replacement key and value")
	}

	if err := client.DeleteVariable(ctx, created.ID); err != nil {
		t.Fatalf("DeleteVariable(%q): %v", created.ID, err)
	}
	deleted = true
	variables, err = collectVariables(ctx, client)
	if err != nil {
		t.Fatalf("ListVariables after delete: %v", err)
	}
	for _, variable := range variables {
		if variable.ID == created.ID {
			t.Error("deleted variable is still listed")
		}
	}
}

func collectVariables(ctx context.Context, client *n8n.Client) ([]n8n.Variable, error) {
	fetch := func(ctx context.Context, pagination n8n.ListOptions) (n8n.Page[n8n.Variable], error) {
		return client.ListVariables(ctx, n8n.ListVariablesOptions{ListOptions: pagination})
	}
	return n8n.Collect(ctx, fetch, n8n.ListOptions{}, 0)
}

// TestVariableListCommand runs read-only listing through the full CLI and
// verifies the environment credential never reaches output.
func TestVariableListCommand(t *testing.T) {
	instance := integration.Require(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"variable", "list", "--limit", "10", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("variable list exit code = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}
	var page n8n.Page[n8n.Variable]
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not a variable page: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
