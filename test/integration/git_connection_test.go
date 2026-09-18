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

// TestGitConnectionList verifies the read-only list through both client and
// CLI. Mutations are never run against the shared instance: create is limited
// to one connection, and push/pull rewrite versioned content, so their
// behavior is covered by the client and CLI tests.
func TestGitConnectionList(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	page, err := client.ListGitConnections(context.Background(), n8n.ListOptions{})
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) || n8n.IsStatus(err, http.StatusServiceUnavailable) {
		t.Skip("integration instance has no GitConnections module")
	}
	if err != nil {
		t.Fatalf("ListGitConnections: %v", err)
	}
	for _, connection := range page.Data {
		if connection.ID == "" || connection.Name == "" || connection.RepositoryURL == "" {
			t.Errorf("connection is missing id, name, or repository URL: %+v", connection)
		}
		if connection.ConnectionType == "" {
			t.Errorf("connection %q is missing its type: %+v", connection.ID, connection)
		}
		if len(page.Data) > 1 {
			t.Errorf("connections = %d, want at most one per instance", len(page.Data))
		}
	}

	if len(page.Data) > 0 {
		got, err := client.GetGitConnection(context.Background(), page.Data[0].ID)
		if err != nil {
			t.Fatalf("GetGitConnection(%q): %v", page.Data[0].ID, err)
		}
		if got.ID != page.Data[0].ID {
			t.Errorf("get ID = %q, want %q", got.ID, page.Data[0].ID)
		}
		projects, err := client.ListGitConnectionProjects(context.Background(), page.Data[0].ID)
		if err != nil {
			t.Fatalf("ListGitConnectionProjects(%q): %v", page.Data[0].ID, err)
		}
		_ = projects
	}

	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"git-connection", "list", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		if strings.Contains(errOut.String(), "403") || strings.Contains(errOut.String(), "404") || strings.Contains(errOut.String(), "503") {
			t.Skip("integration instance has no GitConnections module")
		}
		t.Fatalf("git-connection list exit code = %d (stderr: %s)", code, errOut.String())
	}
	var listPage n8n.Page[n8n.GitConnection]
	if err := json.Unmarshal([]byte(out.String()), &listPage); err != nil {
		t.Fatalf("stdout is not a git-connection page: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("API credential leaked into stdout")
	}
}
