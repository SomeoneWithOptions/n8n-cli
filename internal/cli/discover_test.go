package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// discoverPayload is the shape the instance returns: one {"data": ...} envelope
// holding scopes, resources and filters.
const discoverPayload = `{
  "data": {
    "scopes": ["workflow:read", "workflow:list", "tag:list"],
    "resources": {
      "workflow": {
        "operations": ["read", "list"],
        "endpoints": [
          {"method": "GET", "path": "/api/v1/workflows", "operationId": "getWorkflows"},
          {"method": "GET", "path": "/api/v1/workflows/{id}", "operationId": "getWorkflow"}
        ]
      },
      "tags": {
        "operations": ["list"],
        "endpoints": [{"method": "GET", "path": "/api/v1/tags", "operationId": "getTags"}]
      }
    },
    "filters": {"resource": {"description": "Filter to a specific resource", "values": ["workflow", "tags"]}},
    "specUrl": "/api/v1/openapi.yml"
  }
}`

// discoverOneResource is what the API returns for --resource workflow.
const discoverOneResource = `{
  "data": {
    "scopes": ["workflow:read"],
    "resources": {
      "workflow": {
        "operations": ["read"],
        "endpoints": [{"method": "GET", "path": "/api/v1/workflows/{id}", "operationId": "getWorkflow"}]
      }
    },
    "specUrl": "/api/v1/openapi.yml"
  }
}`

// discoverFixture is a logged-in machine whose instance answers discover calls.
func discoverFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestDiscoverText(t *testing.T) {
	f := discoverFixture(t, discoverPayload)

	got := f.run("discover")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	for _, want := range []string{
		f.server.URL,
		"Context:",
		DefaultContextName,
		"Auth type:",
		string(n8n.AuthAPIKey),
		"Scopes:",
		"3",
		"Resources:",
		"2",
		"/api/v1/openapi.yml",
		"RESOURCE",
		"ENDPOINTS",
		"OPERATIONS",
		"workflow",
		"list, read",
		"tags",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, got.stdout)
		}
	}
	// The endpoint table is only shown for a single resource.
	if strings.Contains(got.stdout, "OPERATION ID") {
		t.Errorf("stdout lists endpoints for an unfiltered run\n%s", got.stdout)
	}
	if !strings.Contains(got.stderr, "--output json") {
		t.Errorf("stderr = %q, want a pointer to the full output", got.stderr)
	}

	req := f.lastRequest()
	if req.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", req.Method)
	}
	if want := n8n.BasePath + n8n.DiscoverPath; req.URL.Path != want {
		t.Errorf("path = %q, want %q", req.URL.Path, want)
	}
	if req.URL.RawQuery != "" {
		t.Errorf("query = %q, want empty for an unfiltered run", req.URL.RawQuery)
	}
	if req.Header.Get(n8n.HeaderAPIKey) != testAPIKey {
		t.Error("the saved credential was not sent")
	}
}

func TestDiscoverTextListsEndpointsForOneResource(t *testing.T) {
	f := discoverFixture(t, discoverOneResource)

	got := f.run("discover", "--resource", "workflow")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	for _, want := range []string{"METHOD", "PATH", "OPERATION ID", "GET", "/api/v1/workflows/{id}", "getWorkflow"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, got.stdout)
		}
	}
	if q := f.lastRequest().URL.RawQuery; q != "resource=workflow" {
		t.Errorf("query = %q, want %q", q, "resource=workflow")
	}
}

func TestDiscoverJSON(t *testing.T) {
	f := discoverFixture(t, discoverPayload)

	got := f.run("discover", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}

	var discovery n8n.Discovery
	if err := json.Unmarshal([]byte(got.stdout), &discovery); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, got.stdout)
	}
	if len(discovery.Scopes) != 3 || discovery.Scopes[0] != "workflow:read" {
		t.Errorf("scopes = %v, want the three from the instance", discovery.Scopes)
	}
	if !discovery.HasOperation("workflow", "list") {
		t.Errorf("resources = %+v, want the workflow list operation", discovery.Resources)
	}
	if discovery.SpecURL != "/api/v1/openapi.yml" {
		t.Errorf("specUrl = %q, want the instance value", discovery.SpecURL)
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want JSON output to be quiet", got.stderr)
	}
}

func TestDiscoverQueryFilters(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "none", args: nil, want: ""},
		{name: "resource", args: []string{"--resource", "workflow"}, want: "resource=workflow"},
		{name: "operation", args: []string{"--operation", "create"}, want: "operation=create"},
		{name: "schemas", args: []string{"--schemas"}, want: "include=schemas"},
		{
			name: "all",
			args: []string{"--resource", "workflow", "--operation", "create", "--schemas"},
			want: "include=schemas&operation=create&resource=workflow",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := discoverFixture(t, discoverPayload)
			got := f.run(append([]string{"discover"}, tt.args...)...)
			if got.code != ExitSuccess {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
			}
			if q := f.lastRequest().URL.RawQuery; q != tt.want {
				t.Errorf("query = %q, want %q", q, tt.want)
			}
		})
	}
}

func TestDiscoverEmptyResult(t *testing.T) {
	f := discoverFixture(t, `{"data":{"scopes":[],"resources":{},"specUrl":"/api/v1/openapi.yml"}}`)

	got := f.run("discover", "--resource", "nope")
	// The API answered; nothing matched. That is information, not a failure.
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if !strings.Contains(got.stderr, "--resource nope") {
		t.Errorf("stderr = %q, want it to repeat the filter that matched nothing", got.stderr)
	}
	if strings.Contains(got.stdout, "RESOURCE") {
		t.Errorf("stdout = %q, want no resource table", got.stdout)
	}
}

func TestDiscoverUsesURLOverride(t *testing.T) {
	f := discoverFixture(t, discoverPayload)

	got := f.run("discover", "--url", f.server.URL)
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if !strings.Contains(got.stdout, f.server.URL) {
		t.Errorf("stdout = %q, want the overridden instance", got.stdout)
	}
}

func TestDiscoverRequiresCredential(t *testing.T) {
	f := newFixture(t)
	f.env[config.EnvURL] = f.server.URL

	got := f.run("discover")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
	if !strings.Contains(got.stderr, "auth login") {
		t.Errorf("stderr = %q, want it to point at 'n8n auth login'", got.stderr)
	}
	if f.requestCount() != 0 {
		t.Error("a request was sent without a credential")
	}
}

func TestDiscoverAPIErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		args    []string
		wantErr []string
	}{
		{
			name:    "unauthorized",
			status:  http.StatusUnauthorized,
			wantErr: []string{"401", "auth login"},
		},
		{
			name:    "forbidden",
			status:  http.StatusForbidden,
			args:    []string{"--resource", "workflow"},
			wantErr: []string{"403", "scope", "n8n discover --resource workflow"},
		},
		{
			name:    "server error",
			status:  http.StatusInternalServerError,
			wantErr: []string{"500"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := discoverFixture(t, discoverPayload)
			f.status = tt.status

			got := f.run(append([]string{"discover"}, tt.args...)...)
			if got.code != ExitError {
				t.Fatalf("exit code = %d, want %d", got.code, ExitError)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty on failure", got.stdout)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want it to contain %q", got.stderr, want)
				}
			}
		})
	}
}

func TestDiscoverUnauthorizedWithEnvironmentCredential(t *testing.T) {
	f := newFixture(t)
	f.env[config.EnvURL] = f.server.URL
	f.env[config.EnvAPIKey] = testAPIKey
	f.status = http.StatusUnauthorized

	got := f.run("discover")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	// Logging in would not fix a credential that came from the environment.
	if !strings.Contains(got.stderr, "environment") {
		t.Errorf("stderr = %q, want it to name the environment as the source", got.stderr)
	}
}

func TestDiscoverRejectsBadFlags(t *testing.T) {
	tests := map[string][]string{
		"unknown output": {"discover", "--output", "yaml"},
		"positional arg": {"discover", "workflow"},
		"unknown flag":   {"discover", "--include", "schemas"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			f := discoverFixture(t, discoverPayload)
			before := f.requestCount()

			got := f.run(args...)
			if got.code != ExitError {
				t.Fatalf("exit code = %d, want %d", got.code, ExitError)
			}
			if got.stderr == "" {
				t.Error("stderr is empty, want an error message")
			}
			if f.requestCount() != before {
				t.Error("an invalid invocation still reached the instance")
			}
		})
	}
}
