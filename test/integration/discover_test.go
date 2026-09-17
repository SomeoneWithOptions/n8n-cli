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

// TestDiscover exercises GET /discover against the live instance. It is
// read-only: discover creates, changes and deletes nothing.
func TestDiscover(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)
	ctx := context.Background()

	discovery, err := client.Discover(ctx, n8n.DiscoverOptions{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(discovery.Scopes) == 0 {
		t.Error("scopes is empty; a working key reports at least one scope")
	}
	if len(discovery.Resources) == 0 {
		t.Fatal("resources is empty; the instance reported no capabilities")
	}
	if discovery.SpecURL == "" {
		t.Error("specUrl is empty; the capability map should point at the OpenAPI document")
	}
	// The endpoint must describe itself.
	if _, ok := discovery.Resource("discover"); !ok {
		t.Errorf("resources = %v, want the discover resource itself", discovery.ResourceNames())
	}
	if _, ok := discovery.Endpoint("GET", n8n.BasePath+n8n.DiscoverPath); !ok {
		t.Errorf("no GET %s endpoint in the capability map", n8n.BasePath+n8n.DiscoverPath)
	}
	for name, resource := range discovery.Resources {
		if len(resource.Endpoints) == 0 {
			t.Errorf("resource %q lists no endpoints", name)
		}
		for _, e := range resource.Endpoints {
			if e.Method == "" || e.Path == "" || e.OperationID == "" {
				t.Errorf("resource %q has an incomplete endpoint: %+v", name, e)
			}
		}
	}
}

func TestDiscoverFilters(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)
	ctx := context.Background()

	full, err := client.Discover(ctx, n8n.DiscoverOptions{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	filter, ok := full.Filters["resource"]
	if !ok || len(filter.Values) == 0 {
		t.Skip("the instance advertises no resource filter values")
	}
	want := filter.Values[0]

	filtered, err := client.Discover(ctx, n8n.DiscoverOptions{Resource: want})
	if err != nil {
		t.Fatalf("Discover(resource=%s): %v", want, err)
	}
	if names := filtered.ResourceNames(); len(names) != 1 || names[0] != want {
		t.Errorf("resources = %v, want only %q", names, want)
	}
}

// TestDiscoverIncludesSchemas checks the include=schemas contract: at least one
// endpoint with a request body comes back with its schema inlined.
func TestDiscoverIncludesSchemas(t *testing.T) {
	instance := integration.Require(t)
	client := instance.Client(t)

	discovery, err := client.Discover(context.Background(), n8n.DiscoverOptions{Include: n8n.DiscoverIncludeSchemas})
	if err != nil {
		t.Fatalf("Discover(include=schemas): %v", err)
	}

	var withBody, withSchema int
	for _, name := range discovery.ResourceNames() {
		for _, e := range discovery.Resources[name].Endpoints {
			if e.Method == "POST" || e.Method == "PUT" || e.Method == "PATCH" {
				withBody++
			}
			if len(e.RequestSchema) > 0 {
				withSchema++
				var schema map[string]any
				if err := json.Unmarshal(e.RequestSchema, &schema); err != nil {
					t.Errorf("%s %s: request schema is not a JSON object: %v", e.Method, e.Path, err)
				}
			}
		}
	}
	if withBody > 0 && withSchema == 0 {
		t.Errorf("%d endpoints take a body but none carried a schema with include=schemas", withBody)
	}
}

func TestDiscoverRejectsBadCredential(t *testing.T) {
	instance := integration.Require(t)

	auth, err := n8n.NewAPIKeyAuth("not-a-valid-api-key")
	if err != nil {
		t.Fatalf("NewAPIKeyAuth: %v", err)
	}
	client, err := n8n.New(instance.URL, n8n.WithAuth(auth))
	if err != nil {
		t.Fatalf("n8n.New: %v", err)
	}
	if _, err := client.Discover(context.Background(), n8n.DiscoverOptions{}); !n8n.IsUnauthorized(err) {
		t.Fatalf("Discover with a bad key = %v, want 401", err)
	}
}

// TestDiscoverCommand runs the CLI itself against the instance, with the
// credential supplied through the environment and a throwaway config
// directory, so nothing on the developer's machine is read or written.
func TestDiscoverCommand(t *testing.T) {
	instance := integration.Require(t)

	env := map[string]string{
		config.EnvURL:    instance.URL,
		config.EnvAPIKey: instance.APIKey.Reveal(),
	}
	var out, errOut strings.Builder
	interactive := false
	code := cli.Run(context.Background(), []string{"discover", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("n8n discover exit code = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}

	var discovery n8n.Discovery
	if err := json.Unmarshal([]byte(out.String()), &discovery); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	if len(discovery.Resources) == 0 {
		t.Error("the CLI reported no resources")
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("the credential leaked into stdout")
	}
}
