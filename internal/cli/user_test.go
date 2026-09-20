package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliUser = `{
  "id":"123e4567-e89b-12d3-a456-426614174000",
  "email":"person@example.com",
  "firstName":"Pat",
  "lastName":"Example",
  "isPending":false,
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z",
  "role":"global:member",
  "mfaEnabled":true
}`

func userFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestUserListTextJSONAndPagination(t *testing.T) {
	f := userFixture(t, `{"data":[`+cliUser+`],"nextCursor":"next/one?x=1"}`)
	got := f.run("user", "list", "--limit", "50", "--offset", "25", "--include-role", "--project-id", "project/one?x=1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Users:", "person@example.com", "Pat Example", "active", "global:member", "MFA", "Next cursor:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	query := req.URL.Query()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.UsersPath || query.Get("limit") != "50" || query.Get("offset") != "25" || query.Get("includeRole") != "true" || query.Get("projectId") != "project/one?x=1" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
	}

	got = f.run("user", "list", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.User]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 || page.Data[0].Email != "person@example.com" {
		t.Errorf("stdout = %q, want user page (error: %v)", got.stdout, err)
	}
}

func TestUserListAllKeepsFiltersAndFollowsCursor(t *testing.T) {
	f := userFixture(t, "")
	f.bodyFunc = func(page int) string {
		if page == 0 {
			return `{"data":[` + cliUser + `],"nextCursor":"next/one?x=1"}`
		}
		pending := strings.Replace(cliUser, `"id":"123e4567-e89b-12d3-a456-426614174000"`, `"id":"two"`, 1)
		pending = strings.Replace(pending, `"email":"person@example.com"`, `"email":"two@example.com"`, 1)
		return `{"data":[` + pending + `],"nextCursor":null}`
	}
	got := f.run("user", "list", "--all", "--offset", "5", "--limit", "1", "--include-role", "--project-id", "project-1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.User]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 2 || page.NextCursor != "" {
		t.Errorf("stdout = %q, want two collected users (error: %v)", got.stdout, err)
	}
	f.mu.Lock()
	requests := append([]*http.Request(nil), f.requests...)
	f.mu.Unlock()
	if len(requests) < 3 {
		t.Fatalf("requests = %d, want login plus two user pages", len(requests))
	}
	first, second := requests[len(requests)-2], requests[len(requests)-1]
	if first.URL.Query().Get("offset") != "5" || first.URL.Query().Get("cursor") != "" {
		t.Errorf("first query = %q", first.URL.RawQuery)
	}
	if second.URL.Query().Get("offset") != "" || second.URL.Query().Get("cursor") != "next/one?x=1" {
		t.Errorf("second query = %q", second.URL.RawQuery)
	}
	for _, req := range []*http.Request{first, second} {
		if req.URL.Query().Get("projectId") != "project-1" || req.URL.Query().Get("includeRole") != "true" || req.URL.Query().Get("limit") != "1" {
			t.Errorf("filters not preserved: %q", req.URL.RawQuery)
		}
	}
}

func TestUserGetByEmailEscapesPath(t *testing.T) {
	f := userFixture(t, cliUser)
	got := f.run("user", "get", "person+ops@example.com/one?x=1", "--include-role")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"User:", "person@example.com", "Pat Example", "Role:", "global:member", "MFA enabled:", "yes"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+"/users/person+ops@example.com%2Fone%3Fx=1" || req.URL.Query().Get("includeRole") != "true" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
	}
}

func TestUserCreatePreservesBulkPartialResults(t *testing.T) {
	response := `[
	  {"user":{"id":"one","email":"one@example.com","inviteAcceptUrl":"https://n8n.example/signup?token=sensitive","emailSent":false,"role":"global:member"},"error":""},
	  {"user":{"id":"two","email":"two@example.com","emailSent":false,"role":"global:member"},"error":"SMTP unavailable"}
	]`
	input := `[{"email":"one@example.com","role":"global:member"},{"email":"two@example.com"}]`
	f := userFixture(t, response)
	f.stdin = input
	got := f.run("user", "create", "--input", "-", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.UsersPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	assertCLIJSON(t, input, f.lastBody())
	var results []n8n.CreateUserResult
	if err := json.Unmarshal([]byte(got.stdout), &results); err != nil || len(results) != 2 || results[0].User == nil || results[0].User.InviteAcceptURL == "" || results[1].Error != "SMTP unavailable" {
		t.Errorf("stdout = %q, want both per-user results (error: %v)", got.stdout, err)
	}

	f.stdin = input
	got = f.run("user", "create", "--input", "-")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "created") || !strings.Contains(got.stdout, "error") || !strings.Contains(got.stdout, "SMTP unavailable") {
		t.Errorf("text result = %+v", got)
	}
}

func TestUserRoleSetAndDelete(t *testing.T) {
	t.Run("role set", func(t *testing.T) {
		f := userFixture(t, "")
		got := f.run("user", "role", "set", "person+ops@example.com/one?x=1", "global:custom/role", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPatch || req.URL.EscapedPath() != n8n.BasePath+"/users/person+ops@example.com%2Fone%3Fx=1/role" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"newRoleName":"global:custom/role"}`, f.lastBody())
		var result userMutation
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Role != "global:custom/role" || result.Identifier == "" {
			t.Errorf("stdout = %q, want role acknowledgement (error: %v)", got.stdout, err)
		}
	})

	t.Run("delete confirmation", func(t *testing.T) {
		f := userFixture(t, "")
		before := f.requestCount()
		f.stdin = "no\n"
		got := f.run("user", "delete", "person@example.com")
		if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || !strings.Contains(got.stderr, "owned personal-project resources") || f.requestCount() != before {
			t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
		}
		f.interactive = false
		got = f.run("user", "delete", "person@example.com")
		if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
			t.Errorf("non-interactive result = %+v", got)
		}
		got = f.run("user", "delete", "person+ops@example.com/one?x=1", "--yes", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodDelete || req.URL.EscapedPath() != n8n.BasePath+"/users/person+ops@example.com%2Fone%3Fx=1" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
	})
}

func TestUserValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{name: "offset with cursor", args: []string{"user", "list", "--offset", "1", "--cursor", "next"}, want: "cannot be used together"},
		{name: "large limit", args: []string{"user", "list", "--limit", "251"}, want: "must not exceed 250"},
		{name: "missing input", args: []string{"user", "create"}, want: "--input is required"},
		{name: "unknown input field", stdin: `[{"email":"one@example.com","name":"One"}]`, args: []string{"user", "create", "--input", "-"}, want: "unknown field"},
		{name: "empty input array", stdin: `[]`, args: []string{"user", "create", "--input", "-"}, want: "at least one"},
		{name: "invalid email", stdin: `[{"email":"not-email"}]`, args: []string{"user", "create", "--input", "-"}, want: "invalid"},
		{name: "multiple documents", stdin: `[] []`, args: []string{"user", "create", "--input", "-"}, want: "multiple documents"},
		{name: "blank get identifier", args: []string{"user", "get", " "}, want: "ID or email is required"},
		{name: "blank role", args: []string{"user", "role", "set", "person@example.com", " "}, want: "role slug is required"},
		{name: "padded delete identifier", args: []string{"user", "delete", " person@example.com", "--yes"}, want: "whitespace"},
		{name: "unknown output", args: []string{"user", "list", "--output", "yaml"}, want: "unknown output format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := userFixture(t, `{"data":[],"nextCursor":null}`)
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

func TestUserOwnerScopeAndNotFoundErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		stdin  string
		want   []string
	}{
		{name: "list unauthorized", status: http.StatusUnauthorized, args: []string{"user", "list"}, want: []string{"401", "auth login"}},
		{name: "list owner only", status: http.StatusForbidden, args: []string{"user", "list"}, want: []string{"403", "instance owner", "discover --resource user"}},
		{name: "get owner only", status: http.StatusForbidden, args: []string{"user", "get", "one@example.com"}, want: []string{"403", "instance owner"}},
		{name: "create forbidden", status: http.StatusForbidden, args: []string{"user", "create", "--input", "-"}, stdin: `[{"email":"one@example.com"}]`, want: []string{"403", "owner", "user:create"}},
		{name: "get not found", status: http.StatusNotFound, args: []string{"user", "get", "missing@example.com"}, want: []string{"404"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := userFixture(t, "")
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
