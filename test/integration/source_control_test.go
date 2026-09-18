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

// TestSourceControlStatus verifies the read-only preview through both client
// and CLI, in both directions. Push mutates the remote Git branch and pull
// rewrites local instance content, so neither is exercised against the shared
// instance; their behavior is covered by the client and CLI tests.
func TestSourceControlStatus(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	for _, direction := range []n8n.SourceControlDirection{n8n.SourceControlDirectionPush, n8n.SourceControlDirectionPull} {
		status, err := client.GetSourceControlStatus(context.Background(), direction)
		if n8n.IsForbidden(err) || n8n.IsNotFound(err) || n8n.IsStatus(err, http.StatusServiceUnavailable) {
			t.Skip("integration instance has no licensed, connected source control")
		}
		if err != nil {
			t.Fatalf("GetSourceControlStatus(%s): %v", direction, err)
		}
		for _, file := range status.Data {
			if file.File == "" || file.ID == "" || file.Name == "" {
				t.Errorf("direction %s: file is missing file, id, or name: %+v", direction, file)
			}
			if file.Type == "" || file.Status == "" || file.Location == "" {
				t.Errorf("direction %s: file %q is missing type, status, or location: %+v", direction, file.ID, file)
			}
		}
	}

	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"source-control", "status", "--direction", "push", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		if strings.Contains(errOut.String(), "403") || strings.Contains(errOut.String(), "404") || strings.Contains(errOut.String(), "503") {
			t.Skip("integration instance has no licensed, connected source control")
		}
		t.Fatalf("source-control status exit code = %d (stderr: %s)", code, errOut.String())
	}
	var status n8n.SourceControlStatus
	if err := json.Unmarshal([]byte(out.String()), &status); err != nil {
		t.Fatalf("stdout is not a source-control status: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("API credential leaked into stdout")
	}
}
