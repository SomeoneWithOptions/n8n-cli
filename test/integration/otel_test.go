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

// TestGetOtelSettings verifies the read-only operation through both client and
// CLI. PUT is not exercised because settings are instance-wide and apply to the
// running process immediately. Test-trace is not exercised because it sends a
// span to the instance's configured external collector.
func TestGetOtelSettings(t *testing.T) {
	instance := integration.Require(t)
	settings, err := instance.Client(t).GetOtelSettings(context.Background())
	if n8n.IsForbidden(err) {
		t.Skip("integration credential lacks otel:manage")
	}
	if err != nil {
		t.Fatalf("GetOtelSettings: %v", err)
	}
	switch settings.ExporterProtocol {
	case n8n.OtelProtocolHTTPProtobuf, n8n.OtelProtocolGRPC:
	default:
		t.Errorf("unexpected exporter protocol %q", settings.ExporterProtocol)
	}

	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"otel", "get", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("otel get exit code = %d (stderr: %s)", code, errOut.String())
	}
	var cliSettings n8n.OtelSettings
	if err := json.Unmarshal([]byte(out.String()), &cliSettings); err != nil {
		t.Fatalf("stdout is not OpenTelemetry settings: %v\n%s", err, out.String())
	}
	if cliSettings.ExporterProtocol != settings.ExporterProtocol || cliSettings.Enabled != settings.Enabled || cliSettings.ExporterEndpoint != settings.ExporterEndpoint {
		t.Errorf("CLI settings differ from client: CLI=%+v client=%+v", cliSettings, settings)
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("API credential leaked into stdout")
	}
}
