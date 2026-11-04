package cli

import (
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliDependencyPage = `{
  "data":[
    {"type":"credential","id":"cred-1","name":"GitHub","projectId":"project-1"},
    {"type":"parentWorkflow","id":"wf-2","name":"Caller"}
  ],
  "nextCursor":"next/one?x=1",
  "inaccessibleCount":3
}`

// dependencyCommands is one entry per dependency lookup: the argv prefix, the
// escaped request path for ID "a/b", and the names its messages must use.
var dependencyCommands = []struct {
	name     string
	args     []string
	escaped  string
	label    string
	scope    string
	list     string
	discover string
}{
	{name: "workflow", args: []string{"workflow", "dependencies"}, escaped: "/workflows/a%2Fb/dependencies", label: "Workflow:", scope: "workflow:read", list: "'n8n workflow list'", discover: "discover --resource workflow"},
	{name: "credential", args: []string{"credential", "dependencies"}, escaped: "/credentials/a%2Fb/dependencies", label: "Credential:", scope: "credential:read", list: "'n8n credential list'", discover: "discover --resource credential"},
	{name: "data table", args: []string{"data-table", "dependencies"}, escaped: "/data-tables/a%2Fb/dependencies", label: "Data table:", scope: "dataTable:read", list: "'n8n data-table list'", discover: "discover --resource datatable"},
}

func dependencyArgs(prefix []string, rest ...string) []string {
	return append(append([]string(nil), prefix...), rest...)
}

func TestDependenciesTextOutputAndRequest(t *testing.T) {
	for _, tc := range dependencyCommands {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.login()
			f.body = cliDependencyPage
			got := f.run(dependencyArgs(tc.args, "a/b", "--limit", "25", "--cursor", "start/one")...)
			if got.code != ExitSuccess {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			for _, want := range []string{
				tc.label, "a/b", "Dependencies:  2", "Inaccessible:  3",
				"RELATION", "uses", "credential", "cred-1", "GitHub", "project-1",
				"used by", "parentWorkflow", "wf-2", "Caller",
				"Next cursor:", "next/one?x=1",
			} {
				if !strings.Contains(got.stdout, want) {
					t.Errorf("stdout missing %q:\n%s", want, got.stdout)
				}
			}
			if !strings.Contains(got.stderr, "3 dependencies are not listed") {
				t.Errorf("stderr = %q, want the inaccessible note", got.stderr)
			}
			req := f.lastRequest()
			if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+tc.escaped {
				t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.EscapedPath(), n8n.BasePath+tc.escaped)
			}
			if q := req.URL.Query(); q.Get("limit") != "25" || q.Get("cursor") != "start/one" {
				t.Errorf("query = %q", req.URL.RawQuery)
			}
		})
	}
}

func TestDependenciesJSONIsTheAPIPage(t *testing.T) {
	for _, tc := range dependencyCommands {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.login()
			f.body = cliDependencyPage
			got := f.run(dependencyArgs(tc.args, "a/b", "--output", "json")...)
			if got.code != ExitSuccess {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			assertCLIJSON(t, cliDependencyPage, got.stdout)
			if got.stderr != "" {
				t.Errorf("stderr = %q, want empty for JSON output", got.stderr)
			}
			if f.lastRequest().URL.RawQuery != "" {
				t.Errorf("query = %q, want none without pagination flags", f.lastRequest().URL.RawQuery)
			}
		})
	}
}

func TestDependenciesEmptyResultHintsOnStderr(t *testing.T) {
	f := newFixture(t)
	f.login()
	f.body = `{"data":[],"nextCursor":null,"inaccessibleCount":0}`
	got := f.run("credential", "dependencies", "cred-1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, "Dependencies:  0") || strings.Contains(got.stdout, "RELATION") || strings.Contains(got.stdout, "Next cursor") {
		t.Errorf("stdout = %q", got.stdout)
	}
	if !strings.Contains(got.stderr, "No dependencies") {
		t.Errorf("stderr = %q, want the empty-result hint", got.stderr)
	}

	got = f.run("credential", "dependencies", "cred-1", "--output", "json")
	assertCLIJSON(t, `{"data":[],"nextCursor":"","inaccessibleCount":0}`, got.stdout)
}

func TestDependenciesAllFollowsCursorAndKeepsInaccessibleCount(t *testing.T) {
	f := newFixture(t)
	f.login()
	f.bodyFunc = func(page int) string {
		if page == 0 {
			return `{"data":[{"type":"parentWorkflow","id":"wf-1","name":"One"}],"nextCursor":"next","inaccessibleCount":4}`
		}
		return `{"data":[{"type":"agent","id":"agent-1","name":"Helper"}],"nextCursor":null,"inaccessibleCount":4}`
	}
	before := f.requestCount()
	got := f.run("workflow", "dependencies", "wf-0", "--all", "--limit", "1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	assertCLIJSON(t, `{
  "data":[
    {"type":"parentWorkflow","id":"wf-1","name":"One"},
    {"type":"agent","id":"agent-1","name":"Helper"}
  ],
  "nextCursor":"",
  "inaccessibleCount":4,
  "collection":{"truncated":false,"limit":10000}
}`, got.stdout)
	if n := f.requestCount() - before; n != 2 {
		t.Fatalf("requests = %d, want 2", n)
	}
	if q := f.lastRequest().URL.Query(); q.Get("cursor") != "next" || q.Get("limit") != "1" {
		t.Errorf("second query = %q, want the returned cursor and the same limit", f.lastRequest().URL.RawQuery)
	}
}

func TestDependenciesErrorsNameScopeAndListCommand(t *testing.T) {
	for _, tc := range dependencyCommands {
		t.Run(tc.name, func(t *testing.T) {
			for _, tt := range []struct {
				status int
				want   []string
			}{
				{status: http.StatusForbidden, want: []string{"403", tc.scope, tc.discover}},
				{status: http.StatusNotFound, want: []string{"404", tc.list, tc.discover}},
				{status: http.StatusUnauthorized, want: []string{"401", "auth login"}},
			} {
				f := newFixture(t)
				f.login()
				f.status = tt.status
				got := f.run(dependencyArgs(tc.args, "id-1")...)
				if got.code != ExitError || got.stdout != "" {
					t.Errorf("%d: result = %+v, want error with empty stdout", tt.status, got)
				}
				for _, want := range tt.want {
					if !strings.Contains(got.stderr, want) {
						t.Errorf("%d: stderr = %q, want %q", tt.status, got.stderr, want)
					}
				}
			}
		})
	}
}

func TestDependenciesValidateBeforeRequest(t *testing.T) {
	for _, tc := range dependencyCommands {
		t.Run(tc.name, func(t *testing.T) {
			for _, tt := range []struct {
				args []string
				want string
			}{
				{args: []string{""}, want: "ID is required"},
				{args: []string{" id-1"}, want: "must not start or end with whitespace"},
				{args: []string{"id-1", "--limit", "251"}, want: "limit must not exceed 250"},
				{args: []string{"id-1", "--limit", "-1"}, want: "limit must not be negative"},
				{args: []string{"id-1", "--output", "yaml"}, want: "output"},
				{args: []string{}, want: "accepts 1 arg"},
			} {
				f := newFixture(t)
				f.login()
				f.body = cliDependencyPage
				before := f.requestCount()
				got := f.run(dependencyArgs(tc.args, tt.args...)...)
				if got.code == ExitSuccess || !strings.Contains(got.stderr, tt.want) {
					t.Errorf("%v: result = %+v, want error containing %q", tt.args, got, tt.want)
				}
				if f.requestCount() != before {
					t.Errorf("%v: invalid invocation reached instance", tt.args)
				}
			}
		})
	}
}

func TestDataTableDependenciesRequiresAPIKey(t *testing.T) {
	f := newFixture(t)
	f.env[config.EnvURL] = f.server.URL
	f.env[config.EnvBearerToken] = "not-an-api-key"
	got := f.run("data-table", "dependencies", "table-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "API-key") {
		t.Errorf("result = %+v, want the API-key requirement", got)
	}
	if f.requestCount() != 0 {
		t.Error("non-API-key command reached instance")
	}
}

func TestDependencyRelation(t *testing.T) {
	for kind, want := range map[string]string{
		"credential": "uses", "dataTable": "uses", "subWorkflow": "uses", "errorWorkflow": "uses",
		"parentWorkflow": "used by", "errorWorkflowParent": "used by", "agent": "used by",
		"somethingNew": "-",
	} {
		if got := dependencyRelation(kind); got != want {
			t.Errorf("dependencyRelation(%q) = %q, want %q", kind, got, want)
		}
	}
}
