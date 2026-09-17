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

const cliProject = `{"id":"pr-1","name":"Billing automation","type":"team"}`

const cliProjectMember = `{
  "id":"123e4567-e89b-12d3-a456-426614174000",
  "email":"person@example.com",
  "firstName":"Pat",
  "lastName":"Example",
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z",
  "role":"project:viewer"
}`

func projectFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestProjectListTextJSONAndPagination(t *testing.T) {
	f := projectFixture(t, `{"data":[`+cliProject+`],"nextCursor":"next/one?x=1"}`)
	got := f.run("project", "list", "--limit", "50", "--cursor", "prev/one?x=1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Projects:", "pr-1", "Billing automation", "team", "Next cursor:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	query := req.URL.Query()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.ProjectsPath || query.Get("limit") != "50" || query.Get("cursor") != "prev/one?x=1" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
	}

	got = f.run("project", "list", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.Project]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 || page.Data[0].ID != "pr-1" {
		t.Errorf("stdout = %q, want project page (error: %v)", got.stdout, err)
	}
}

func TestProjectListAllFollowsCursor(t *testing.T) {
	f := projectFixture(t, "")
	f.bodyFunc = func(page int) string {
		if page == 0 {
			return `{"data":[` + cliProject + `],"nextCursor":"next/one?x=1"}`
		}
		return `{"data":[{"id":"pr-2","name":"Second","type":"team"}],"nextCursor":null}`
	}
	got := f.run("project", "list", "--all", "--limit", "1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.Project]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 2 || page.NextCursor != "" {
		t.Errorf("stdout = %q, want two collected projects (error: %v)", got.stdout, err)
	}
	f.mu.Lock()
	requests := append([]*http.Request(nil), f.requests...)
	f.mu.Unlock()
	first, second := requests[len(requests)-2], requests[len(requests)-1]
	if first.URL.Query().Get("cursor") != "" || second.URL.Query().Get("cursor") != "next/one?x=1" {
		t.Errorf("cursors = %q then %q", first.URL.RawQuery, second.URL.RawQuery)
	}
	if second.URL.Query().Get("limit") != "1" {
		t.Errorf("page size not preserved: %q", second.URL.RawQuery)
	}
}

func TestProjectListEmptyExplainsNextStep(t *testing.T) {
	f := projectFixture(t, `{"data":[],"nextCursor":null}`)
	got := f.run("project", "list")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "n8n project create") {
		t.Errorf("result = %+v, want a create hint on stderr", got)
	}
}

func TestProjectCreateUpdateAndDelete(t *testing.T) {
	t.Run("create returns project", func(t *testing.T) {
		f := projectFixture(t, cliProject)
		got := f.run("project", "create", "Billing automation", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.ProjectsPath {
			t.Errorf("request = %s %s", req.Method, req.URL.Path)
		}
		assertCLIJSON(t, `{"name":"Billing automation"}`, f.lastBody())
		var project n8n.Project
		if err := json.Unmarshal([]byte(got.stdout), &project); err != nil || project.ID != "pr-1" {
			t.Errorf("stdout = %q, want created project (error: %v)", got.stdout, err)
		}
	})

	t.Run("create without response body", func(t *testing.T) {
		f := projectFixture(t, "")
		got := f.run("project", "create", "Billing automation")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if !strings.Contains(got.stdout, "Billing automation") || !strings.Contains(got.stderr, "n8n project list") {
			t.Errorf("result = %+v, want the name echoed and a list hint", got)
		}
	})

	t.Run("update replaces name", func(t *testing.T) {
		f := projectFixture(t, "")
		got := f.run("project", "update", "pr/one?x=1", "--name", "Billing", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+"/projects/pr%2Fone%3Fx=1" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"name":"Billing"}`, f.lastBody())
		var project n8n.Project
		if err := json.Unmarshal([]byte(got.stdout), &project); err != nil || project.Name != "Billing" || project.ID != "pr/one?x=1" {
			t.Errorf("stdout = %q, want renamed project (error: %v)", got.stdout, err)
		}
	})

	t.Run("delete confirmation", func(t *testing.T) {
		f := projectFixture(t, "")
		before := f.requestCount()
		f.stdin = "no\n"
		got := f.run("project", "delete", "pr-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || !strings.Contains(got.stderr, "credentials") || f.requestCount() != before {
			t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
		}
		f.interactive = false
		got = f.run("project", "delete", "pr-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
			t.Errorf("non-interactive result = %+v", got)
		}
		got = f.run("project", "delete", "pr/one?x=1", "--yes", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodDelete || req.URL.EscapedPath() != n8n.BasePath+"/projects/pr%2Fone%3Fx=1" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		var result projectMutation
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Action != "deleted" || result.ProjectID == "" {
			t.Errorf("stdout = %q, want delete acknowledgement (error: %v)", got.stdout, err)
		}
	})
}

func TestProjectUserListTextAndJSON(t *testing.T) {
	f := projectFixture(t, `{"data":[`+cliProjectMember+`],"nextCursor":"next"}`)
	got := f.run("project", "user", "list", "pr/one?x=1", "--limit", "10")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Members:", "person@example.com", "Pat Example", "project:viewer", "Next cursor:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+"/projects/pr%2Fone%3Fx=1/users" || req.URL.Query().Get("limit") != "10" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
	}

	got = f.run("project", "user", "list", "pr-1", "--output", "json")
	var page n8n.Page[n8n.ProjectMember]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 || page.Data[0].Role != "project:viewer" {
		t.Errorf("stdout = %q, want member page (error: %v)", got.stdout, err)
	}
}

func TestProjectUserAddFromFlagsAndInput(t *testing.T) {
	t.Run("user pairs", func(t *testing.T) {
		f := projectFixture(t, "")
		got := f.run("project", "user", "add", "pr/one?x=1",
			"--user", "us-1=project:viewer", "--user", "us-2=project:editor", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+"/projects/pr%2Fone%3Fx=1/users" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"relations":[{"userId":"us-1","role":"project:viewer"},{"userId":"us-2","role":"project:editor"}]}`, f.lastBody())
		var result projectMutation
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || len(result.Members) != 2 {
			t.Errorf("stdout = %q, want both members echoed (error: %v)", got.stdout, err)
		}
	})

	t.Run("stdin JSON", func(t *testing.T) {
		f := projectFixture(t, "")
		f.stdin = `[{"userId":"us-1","role":"project:admin"}]`
		got := f.run("project", "user", "add", "pr-1", "--input", "-")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"relations":[{"userId":"us-1","role":"project:admin"}]}`, f.lastBody())
		if !strings.Contains(got.stdout, "us-1") || !strings.Contains(got.stdout, "project:admin") {
			t.Errorf("stdout = %q, want the member listed", got.stdout)
		}
	})

	t.Run("file JSON", func(t *testing.T) {
		f := projectFixture(t, "")
		path := filepath.Join(t.TempDir(), "members.json")
		if err := os.WriteFile(path, []byte(`[{"userId":"us-9","role":"project:viewer"}]`), 0o600); err != nil {
			t.Fatalf("write member input: %v", err)
		}
		got := f.run("project", "user", "add", "pr-1", "--input", path)
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"relations":[{"userId":"us-9","role":"project:viewer"}]}`, f.lastBody())
	})
}

func TestProjectUserRoleSetAndRemove(t *testing.T) {
	t.Run("role set", func(t *testing.T) {
		f := projectFixture(t, "")
		got := f.run("project", "user", "role", "set", "pr/one?x=1", "us/two?y=2", "project:admin", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPatch || req.URL.EscapedPath() != n8n.BasePath+"/projects/pr%2Fone%3Fx=1/users/us%2Ftwo%3Fy=2" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"role":"project:admin"}`, f.lastBody())
		var result projectMutation
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Role != "project:admin" || result.UserID == "" {
			t.Errorf("stdout = %q, want role acknowledgement (error: %v)", got.stdout, err)
		}
	})

	t.Run("remove confirmation", func(t *testing.T) {
		f := projectFixture(t, "")
		before := f.requestCount()
		f.stdin = "no\n"
		got := f.run("project", "user", "remove", "pr-1", "us-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || !strings.Contains(got.stderr, "account itself stays") || f.requestCount() != before {
			t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
		}
		f.interactive = false
		got = f.run("project", "user", "remove", "pr-1", "us-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
			t.Errorf("non-interactive result = %+v", got)
		}
		got = f.run("project", "user", "remove", "pr/one?x=1", "us/two?y=2", "--yes")
		if got.code != ExitSuccess {
			t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodDelete || req.URL.EscapedPath() != n8n.BasePath+"/projects/pr%2Fone%3Fx=1/users/us%2Ftwo%3Fy=2" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
	})
}

func TestProjectValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{name: "large limit", args: []string{"project", "list", "--limit", "251"}, want: "must not exceed 250"},
		{name: "unknown output", args: []string{"project", "list", "--output", "yaml"}, want: "unknown output format"},
		{name: "blank create name", args: []string{"project", "create", " "}, want: "project name is required"},
		{name: "missing update name", args: []string{"project", "update", "pr-1"}, want: "--name is required"},
		{name: "padded update name", args: []string{"project", "update", "pr-1", "--name", " Billing"}, want: "whitespace"},
		{name: "padded delete ID", args: []string{"project", "delete", " pr-1", "--yes"}, want: "whitespace"},
		{name: "blank member list project", args: []string{"project", "user", "list", " "}, want: "project ID is required"},
		{name: "member add without members", args: []string{"project", "user", "add", "pr-1"}, want: "--user or --input is required"},
		{name: "member add with both sources", stdin: `[]`, args: []string{"project", "user", "add", "pr-1", "--user", "us-1=project:viewer", "--input", "-"}, want: "cannot be used together"},
		{name: "member add malformed pair", args: []string{"project", "user", "add", "pr-1", "--user", "us-1"}, want: "USER_ID=ROLE"},
		{name: "member add empty role", args: []string{"project", "user", "add", "pr-1", "--user", "us-1="}, want: "project role is required"},
		{name: "member add empty array", stdin: `[]`, args: []string{"project", "user", "add", "pr-1", "--input", "-"}, want: "at least one"},
		{name: "member add unknown field", stdin: `[{"userId":"us-1","role":"project:viewer","extra":1}]`, args: []string{"project", "user", "add", "pr-1", "--input", "-"}, want: "unknown field"},
		{name: "member add multiple documents", stdin: `[] []`, args: []string{"project", "user", "add", "pr-1", "--input", "-"}, want: "multiple documents"},
		{name: "blank role slug", args: []string{"project", "user", "role", "set", "pr-1", "us-1", " "}, want: "project role slug is required"},
		{name: "blank removed user", args: []string{"project", "user", "remove", "pr-1", " ", "--yes"}, want: "user ID is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := projectFixture(t, `{"data":[],"nextCursor":null}`)
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

func TestProjectLicenseScopeAndNotFoundErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		want   []string
	}{
		{name: "list unauthorized", status: http.StatusUnauthorized, args: []string{"project", "list"}, want: []string{"401", "auth login"}},
		{name: "list unlicensed", status: http.StatusForbidden, args: []string{"project", "list"}, want: []string{"403", "not licensed for projects", "discover --resource projects"}},
		{name: "member list forbidden", status: http.StatusForbidden, args: []string{"project", "user", "list", "pr-1"}, want: []string{"403", "user:list"}},
		{name: "member add forbidden", status: http.StatusForbidden, args: []string{"project", "user", "add", "pr-1", "--user", "us-1=project:viewer"}, want: []string{"403", "project scope"}},
		{name: "update not found", status: http.StatusNotFound, args: []string{"project", "update", "pr-1", "--name", "Billing"}, want: []string{"404"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := projectFixture(t, "")
			f.status = tt.status
			f.interactive = false
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
