package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliTag = `{
  "id":"tag-id",
  "name":"Production",
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z"
}`

func tagFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestTagListTextAndJSON(t *testing.T) {
	f := tagFixture(t, `{"data":[`+cliTag+`],"nextCursor":"next page"}`)

	got := f.run("tag", "list", "--limit", "25", "--cursor", "start")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Tags:", "tag-id", "Production", "Next cursor:", "next page"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.TagsPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, n8n.BasePath+n8n.TagsPath)
	}
	if req.URL.Query().Get("limit") != "25" || req.URL.Query().Get("cursor") != "start" {
		t.Errorf("query = %q, want the pagination parameters", req.URL.RawQuery)
	}

	got = f.run("tag", "list", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.Tag]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil {
		t.Fatalf("stdout is not a tag page: %v\n%s", err, got.stdout)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "tag-id" || page.NextCursor != "next page" {
		t.Errorf("page = %+v", page)
	}
}

// TestTagListEmptyPageExplainsItself keeps an empty result actionable and off
// stdout, so JSON consumers still read a valid page.
func TestTagListEmptyPageExplainsItself(t *testing.T) {
	f := tagFixture(t, `{"data":[],"nextCursor":null}`)
	got := f.run("tag", "list")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stderr, "tag create") {
		t.Errorf("stderr = %q, want the next command to run", got.stderr)
	}
	if strings.Contains(got.stdout, "ID") {
		t.Errorf("stdout = %q, want no table header for an empty page", got.stdout)
	}
}

// TestTagListAllFollowsCursors covers --all end to end: two pages in, one list
// of tags out, with the second request carrying the cursor.
func TestTagListAllFollowsCursors(t *testing.T) {
	f := tagFixture(t, "")
	pages := []string{
		`{"data":[{"id":"1","name":"first"}],"nextCursor":"page two"}`,
		`{"data":[{"id":"2","name":"second"}],"nextCursor":null}`,
	}
	f.body = pages[0]
	f.bodyFunc = func(n int) string {
		if n < len(pages) {
			return pages[n]
		}
		return pages[len(pages)-1]
	}

	got := f.run("tag", "list", "--all", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.Tag]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil {
		t.Fatalf("stdout is not a tag page: %v\n%s", err, got.stdout)
	}
	if len(page.Data) != 2 || page.Data[0].ID != "1" || page.Data[1].ID != "2" {
		t.Errorf("collected = %+v, want both pages", page.Data)
	}
	if got := f.lastRequest().URL.Query().Get("cursor"); got != "page two" {
		t.Errorf("last request cursor = %q, want %q", got, "page two")
	}
}

func TestTagCreateGetAndUpdate(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		f := tagFixture(t, cliTag)
		got := f.run("tag", "create", "Production")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if !strings.Contains(got.stdout, "Created:") || !strings.Contains(got.stdout, "Production") {
			t.Errorf("stdout = %q", got.stdout)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.TagsPath {
			t.Errorf("request = %s %s", req.Method, req.URL.Path)
		}
		assertCLIJSON(t, `{"name":"Production"}`, f.lastBody())
	})

	t.Run("get", func(t *testing.T) {
		f := tagFixture(t, cliTag)
		got := f.run("tag", "get", "tag/id?x=1", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		var tag n8n.Tag
		if err := json.Unmarshal([]byte(got.stdout), &tag); err != nil || tag.Name != "Production" {
			t.Errorf("stdout = %q (error: %v)", got.stdout, err)
		}
		req := f.lastRequest()
		if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+"/tags/tag%2Fid%3Fx=1" {
			t.Errorf("request = %s %s, want the ID escaped as one segment", req.Method, req.URL.EscapedPath())
		}
	})

	t.Run("update", func(t *testing.T) {
		f := tagFixture(t, cliTag)
		got := f.run("tag", "update", "tag-id", "--name", "Staging")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if !strings.Contains(got.stdout, "Updated:") {
			t.Errorf("stdout = %q", got.stdout)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPut || req.URL.Path != n8n.BasePath+"/tags/tag-id" {
			t.Errorf("request = %s %s", req.Method, req.URL.Path)
		}
		assertCLIJSON(t, `{"name":"Staging"}`, f.lastBody())
	})
}

func TestTagDeleteRequiresConfirmation(t *testing.T) {
	f := tagFixture(t, cliTag)

	before := f.requestCount()
	f.stdin = "no\n"
	got := f.run("tag", "delete", "tag-id")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
		t.Errorf("declined result = %+v", got)
	}
	if f.requestCount() != before {
		t.Error("a declined delete reached the instance")
	}
	if !strings.Contains(got.stderr, "every workflow") {
		t.Errorf("prompt = %q, want the consequence stated", got.stderr)
	}

	f.interactive = false
	got = f.run("tag", "delete", "tag-id")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("tag", "delete", "tag-id", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	var tag n8n.Tag
	if err := json.Unmarshal([]byte(got.stdout), &tag); err != nil || tag.ID != "tag-id" {
		t.Errorf("stdout = %q (error: %v)", got.stdout, err)
	}
	if req := f.lastRequest(); req.Method != http.MethodDelete || req.URL.Path != n8n.BasePath+"/tags/tag-id" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestTagValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "negative limit", args: []string{"tag", "list", "--limit", "-1"}, want: "negative"},
		{name: "unknown output", args: []string{"tag", "list", "--output", "yaml"}, want: "unknown output format"},
		{name: "blank create name", args: []string{"tag", "create", "  "}, want: "tag name is required"},
		{name: "padded create name", args: []string{"tag", "create", "Production "}, want: "whitespace"},
		{name: "missing update name", args: []string{"tag", "update", "tag-id"}, want: "--name is required"},
		{name: "blank update name", args: []string{"tag", "update", "tag-id", "--name", " "}, want: "--name is required"},
		{name: "blank get ID", args: []string{"tag", "get", " "}, want: "tag ID is required"},
		{name: "blank delete ID", args: []string{"tag", "delete", " ", "--yes"}, want: "tag ID is required"},
		{name: "missing create argument", args: []string{"tag", "create"}, want: "accepts 1 arg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tagFixture(t, cliTag)
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want an error containing %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("an invalid invocation reached the instance")
			}
		})
	}
}

func TestTagWriteConflictAndScopeErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		want   []string
	}{
		{name: "create conflict", status: http.StatusConflict, args: []string{"tag", "create", "Production"},
			want: []string{"409", `"Production"`, "unique", "tag list"}},
		{name: "update conflict", status: http.StatusConflict, args: []string{"tag", "update", "tag-id", "--name", "Staging"},
			want: []string{"409", `"Staging"`, "unique"}},
		{name: "list unauthorized", status: http.StatusUnauthorized, args: []string{"tag", "list"},
			want: []string{"401", "auth login"}},
		{name: "list forbidden", status: http.StatusForbidden, args: []string{"tag", "list"},
			want: []string{"403", "discover --resource tag"}},
		{name: "get not found", status: http.StatusNotFound, args: []string{"tag", "get", "missing"},
			want: []string{"404"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tagFixture(t, cliTag)
			f.status = tt.status
			got := f.run(tt.args...)
			if got.code != ExitError || got.stdout != "" {
				t.Errorf("result = %+v, want a failure with empty stdout", got)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want %q", got.stderr, want)
				}
			}
		})
	}
}
