package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

func skipLogStreamUnavailable(t *testing.T, err error) {
	t.Helper()
	if n8n.IsForbidden(err) || n8n.IsNotFound(err) || n8n.IsStatus(err, 503) {
		t.Skip("Log Streaming license, module, or required scope unavailable")
	}
}

func TestLogStreamRead(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)
	ctx := context.Background()
	_, err := client.GetLogStreamEventTypes(ctx)
	skipLogStreamUnavailable(t, err)
	if err != nil {
		t.Fatal(err)
	}
	destinations, err := client.ListLogStreamDestinations(ctx)
	skipLogStreamUnavailable(t, err)
	if err != nil {
		t.Fatal(err)
	}
	if len(destinations.Data) > 0 {
		_, err = client.GetLogStreamDestination(ctx, destinations.Data[0].ID)
		skipLogStreamUnavailable(t, err)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"log-stream", "event-type", "list", "--output", "json"}, {"log-stream", "destination", "list", "--output", "json"}} {
		var out, errOut strings.Builder
		interactive := false
		env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
		code := cli.Run(ctx, args, cli.Options{
			Streams: cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut}, Version: version.Get(),
			ConfigDir: t.TempDir(), Env: func(name string) string { return env[name] }, Keyring: config.NewMemoryStore(config.StorageKeyring), Interactive: &interactive,
		})
		if code != cli.ExitSuccess {
			t.Fatalf("log-stream exit=%d stderr=%s", code, errOut.String())
		}
		if !json.Valid([]byte(out.String())) {
			t.Fatal("CLI output is not JSON")
		}
		if strings.Contains(out.String(), instance.APIKey.Reveal()) {
			t.Error("API key leaked")
		}
	}
}

// Requires destructive opt-in AND a protected destination JSON file supplied
// through N8N_INTEGRATION_LOG_STREAM_INPUT. Use a disposable instance/receiver.
// Force disabled with no event subscriptions; never alter existing destinations.
// Sending a test message additionally requires N8N_INTEGRATION_LOG_STREAM_TEST=1.
func TestLogStreamLifecycle(t *testing.T) {
	instance := integration.RequireDestructive(t)
	path := os.Getenv("N8N_INTEGRATION_LOG_STREAM_INPUT")
	if path == "" {
		t.Skip("set N8N_INTEGRATION_LOG_STREAM_INPUT to a protected destination JSON file for a disposable receiver")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		t.Fatal("invalid destination input JSON (details redacted)")
	}
	fields["enabled"] = json.RawMessage(`false`)
	fields["subscribedEvents"] = json.RawMessage(`[]`)
	fields["label"], _ = json.Marshal(integration.ResourcePrefix + "log-stream-" + time.Now().UTC().Format("20060102T150405.000000000"))
	raw, err = json.Marshal(fields)
	if err != nil {
		t.Fatal("cannot encode destination input")
	}
	var request n8n.LogStreamDestinationRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	client := instance.Client(t)
	ctx := context.Background()
	created, err := client.CreateLogStreamDestination(ctx, request)
	skipLogStreamUnavailable(t, err)
	if n8n.IsConflict(err) {
		t.Skip("destinations are environment-managed")
	}
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("created destination has no ID")
	}
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := client.DeleteLogStreamDestination(ctx, created.ID); err != nil {
			t.Errorf("cleanup destination %s: %v", created.ID, err)
		}
	})
	got, err := client.GetLogStreamDestination(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID {
		t.Fatal("retrieved wrong destination")
	}
	fields["label"], _ = json.Marshal(integration.ResourcePrefix + "log-stream-updated")
	raw, _ = json.Marshal(fields)
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	updated, err := client.UpdateLogStreamDestination(ctx, created.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Label != integration.ResourcePrefix+"log-stream-updated" {
		t.Error("label update not applied")
	}
	t.Run("test-message", func(t *testing.T) {
		if os.Getenv("N8N_INTEGRATION_LOG_STREAM_TEST") != "1" {
			t.Skip("set N8N_INTEGRATION_LOG_STREAM_TEST=1 to send a test message to the disposable receiver")
		}
		result, err := client.TestLogStreamDestination(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Success {
			t.Error("test message was not delivered")
		}
	})
	if _, err := client.DeleteLogStreamDestination(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	deleted = true
}
