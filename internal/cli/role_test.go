package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliRole = `{
  "slug":"global:custom/one?x=1",
  "displayName":"Workflow auditor",
  "description":"Can inspect workflows",
  "systemRole":false,
  "roleType":"global",
  "scopes":["workflow:list","workflow:read"],
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z",
  "licensed":false,
  "usedByUsers":0,
  "usedByProjects":2
}`

func roleFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func writeRoleInput(t *testing.T, f *fixture, name, body string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(f.dir), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write role input: %v", err)
	}
	return path
}

func TestRoleListTextAndJSONSurfaceLicensing(t *testing.T) {
	projectRole := strings.ReplaceAll(strings.ReplaceAll(cliRole, "global:custom/one?x=1", "project:custom"), `"roleType":"global"`, `"roleType":"project"`)
	projectRole = strings.Replace(projectRole, `"licensed":false`, `"licensed":true`, 1)
	f := roleFixture(t, `{"global":[`+cliRole+`],"project":[`+projectRole+`]}`)

	got := f.run("role", "list", "--with-usage-count")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Roles:", "global:custom/one?x=1", "project:custom", "LICENSED", "no", "yes", "USERS", "PROJECTS"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.RolesPath || req.URL.Query().Get("withUsageCount") != "true" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
	}

	got = f.run("role", "list", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var roles n8n.Roles
	if err := json.Unmarshal([]byte(got.stdout), &roles); err != nil || len(roles.Global) != 1 || roles.Global[0].Licensed == nil || *roles.Global[0].Licensed {
		t.Errorf("stdout = %q, want grouped roles with licensed=false (error: %v)", got.stdout, err)
	}
	if query := f.lastRequest().URL.RawQuery; query != "" {
		t.Errorf("default list query = %q, want empty", query)
	}
}

func TestRoleGetEscapesSlugAndShowsDetails(t *testing.T) {
	f := roleFixture(t, cliRole)
	got := f.run("role", "get", "global:custom/one?x=1", "--with-usage-count")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Role:", "Workflow auditor", "Licensed:", "no", "System role:", "workflow:read", "Used by users:", "0", "Used by projects:", "2"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+"/roles/global:custom%2Fone%3Fx=1" || req.URL.Query().Get("withUsageCount") != "true" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
	}
}

func TestRoleCreateFromStdinAndUpdateFromFile(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		input := `{"displayName":"Workflow auditor","description":"Can inspect workflows","roleType":"global","scopes":["workflow:list","workflow:read"]}`
		f := roleFixture(t, cliRole)
		f.interactive = false
		f.stdin = input
		got := f.run("role", "create", "--input", "-", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.RolesPath {
			t.Errorf("request = %s %s", req.Method, req.URL.Path)
		}
		assertCLIJSON(t, input, f.lastBody())
		var role n8n.Role
		if err := json.Unmarshal([]byte(got.stdout), &role); err != nil || role.Slug == "" {
			t.Errorf("stdout = %q, want role JSON (error: %v)", got.stdout, err)
		}
	})

	t.Run("full replacement with null description and empty scopes", func(t *testing.T) {
		input := `{"displayName":"Workflow auditor","description":null,"scopes":[]}`
		f := roleFixture(t, cliRole)
		path := writeRoleInput(t, f, "role-update.json", input)
		got := f.run("role", "update", "global:custom/one?x=1", "--input", path)
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+"/roles/global:custom%2Fone%3Fx=1" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, input, f.lastBody())
	})
}

func TestRoleDeleteRequiresConfirmationAndEncodesReassignment(t *testing.T) {
	f := roleFixture(t, cliRole)
	before := f.requestCount()
	f.stdin = "no\n"
	got := f.run("role", "delete", "global:custom")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "Assigned users") {
		t.Errorf("prompt = %q, want assigned-user consequence", got.stderr)
	}

	f.interactive = false
	got = f.run("role", "delete", "global:custom")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("role", "delete", "global:custom/one?x=1", "--reassign-role", "global:member/limited?x=2", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodDelete || req.URL.EscapedPath() != n8n.BasePath+"/roles/global:custom%2Fone%3Fx=1" || req.URL.Query().Get("reassignRoleSlug") != "global:member/limited?x=2" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
	}
}

func TestRoleInputAndArgumentValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{name: "missing create input", args: []string{"role", "create"}, want: "--input is required"},
		{name: "unknown create field", stdin: `{"displayName":"Auditor","roleType":"global","scopes":[],"slug":"not-writable"}`, args: []string{"role", "create", "--input", "-"}, want: "unknown field"},
		{name: "invalid create type", stdin: `{"displayName":"Auditor","roleType":"team","scopes":[]}`, args: []string{"role", "create", "--input", "-"}, want: "role type"},
		{name: "missing create scopes", stdin: `{"displayName":"Auditor","roleType":"global"}`, args: []string{"role", "create", "--input", "-"}, want: "scopes are required"},
		{name: "missing update description", stdin: `{"displayName":"Auditor","scopes":[]}`, args: []string{"role", "update", "custom", "--input", "-"}, want: "description is required"},
		{name: "multiple documents", stdin: `{} {}`, args: []string{"role", "create", "--input", "-"}, want: "multiple documents"},
		{name: "blank get slug", args: []string{"role", "get", " "}, want: "role slug is required"},
		{name: "padded update slug", stdin: `{"displayName":"Auditor","description":null,"scopes":[]}`, args: []string{"role", "update", " custom", "--input", "-"}, want: "whitespace"},
		{name: "blank reassignment", args: []string{"role", "delete", "custom", "--reassign-role", " ", "--yes"}, want: "reassign-role"},
		{name: "unknown output", args: []string{"role", "list", "--output", "yaml"}, want: "unknown output format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := roleFixture(t, `{"global":[],"project":[]}`)
			f.interactive = false
			f.stdin = tt.stdin
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want error containing %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("invalid invocation reached instance")
			}
		})
	}
}

func TestRoleImmutableAndScopeErrors(t *testing.T) {
	update := `{"displayName":"Owner","description":null,"scopes":[]}`
	tests := []struct {
		name   string
		status int
		args   []string
		stdin  string
		want   []string
	}{
		{name: "list unauthorized", status: http.StatusUnauthorized, args: []string{"role", "list"}, want: []string{"401", "auth login"}},
		{name: "list forbidden", status: http.StatusForbidden, args: []string{"role", "list"}, want: []string{"403", "discover --resource role"}},
		{name: "get not found", status: http.StatusNotFound, args: []string{"role", "get", "missing"}, want: []string{"404"}},
		{name: "update built-in", status: http.StatusBadRequest, args: []string{"role", "update", "global:owner", "--input", "-"}, stdin: update, want: []string{"400", "built-in/system roles", "immutable", "role get"}},
		{name: "delete built-in", status: http.StatusBadRequest, args: []string{"role", "delete", "global:owner", "--yes"}, want: []string{"400", "built-in/system roles", "immutable", "role get"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := roleFixture(t, `{"global":[],"project":[]}`)
			f.status = tt.status
			f.interactive = false
			f.stdin = tt.stdin
			got := f.run(tt.args...)
			if got.code != ExitError || got.stdout != "" {
				t.Errorf("result = %+v, want failure with empty stdout", got)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want %q", got.stderr, want)
				}
			}
		})
	}
}
