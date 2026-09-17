package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliVariable = `{
  "id":"variable-id",
  "key":"API_HOST",
  "value":"https://api.example.com",
  "type":"string",
  "project":{"id":"project-one","name":"Production","type":"team"}
}`

func variableFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestVariableListTextAndJSON(t *testing.T) {
	f := variableFixture(t, `{"data":[`+cliVariable+`],"nextCursor":"next page"}`)

	got := f.run("variable", "list", "--limit", "25", "--cursor", "start", "--project-id", "project/one", "--state", "empty")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Variables:", "variable-id", "API_HOST", `"https://api.example.com"`, "Production (project-one)", "Next cursor:", "next page"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.VariablesPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, n8n.BasePath+n8n.VariablesPath)
	}
	query := req.URL.Query()
	if query.Get("limit") != "25" || query.Get("cursor") != "start" || query.Get("projectId") != "project/one" || query.Get("state") != "empty" {
		t.Errorf("query = %q, want all list options", req.URL.RawQuery)
	}

	got = f.run("variable", "list", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.Variable]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil {
		t.Fatalf("stdout is not a variable page: %v\n%s", err, got.stdout)
	}
	if len(page.Data) != 1 || page.Data[0].Value != "https://api.example.com" || page.NextCursor != "next page" {
		t.Errorf("page = %+v", page)
	}
}

func TestVariableListQuotesMultilineValues(t *testing.T) {
	f := variableFixture(t, `{"data":[{"id":"id","key":"MULTILINE","value":"first\nsecond\tcolumn"}],"nextCursor":null}`)
	got := f.run("variable", "list")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, `"first\nsecond\tcolumn"`) {
		t.Errorf("stdout = %q, want a safely quoted value", got.stdout)
	}
	if strings.Contains(got.stdout, "first\nsecond") {
		t.Errorf("stdout contains a literal newline from the value: %q", got.stdout)
	}
}

func TestVariableListEmptyPageExplainsItself(t *testing.T) {
	f := variableFixture(t, `{"data":[],"nextCursor":null}`)
	got := f.run("variable", "list")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stderr, "variable create") {
		t.Errorf("stderr = %q, want the next command to run", got.stderr)
	}
	if strings.Contains(got.stdout, "ID") {
		t.Errorf("stdout = %q, want no table header for an empty page", got.stdout)
	}
}

func TestVariableListAllKeepsFilters(t *testing.T) {
	f := variableFixture(t, "")
	pages := []string{
		`{"data":[{"id":"1","key":"FIRST","value":""}],"nextCursor":"page two"}`,
		`{"data":[{"id":"2","key":"SECOND","value":""}],"nextCursor":null}`,
	}
	f.body = pages[0]
	f.bodyFunc = func(n int) string {
		if n < len(pages) {
			return pages[n]
		}
		return pages[len(pages)-1]
	}

	got := f.run("variable", "list", "--all", "--project-id", "project-one", "--state", "empty", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.Variable]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil {
		t.Fatalf("stdout is not a variable page: %v\n%s", err, got.stdout)
	}
	if len(page.Data) != 2 || page.Data[0].ID != "1" || page.Data[1].ID != "2" {
		t.Errorf("collected = %+v, want both pages", page.Data)
	}
	query := f.lastRequest().URL.Query()
	if query.Get("cursor") != "page two" || query.Get("projectId") != "project-one" || query.Get("state") != "empty" {
		t.Errorf("last query = %q, want cursor and persistent filters", f.lastRequest().URL.RawQuery)
	}
}

func TestVariableCreateAndUpdate(t *testing.T) {
	const submitted = "value-that-must-not-be-repeated"
	t.Run("create", func(t *testing.T) {
		f := variableFixture(t, "")
		f.status = http.StatusCreated
		got := f.run("variable", "create", "API_HOST", "--value", submitted, "--project-id", "project-one", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if strings.Contains(got.stdout, submitted) || strings.Contains(got.stderr, submitted) {
			t.Errorf("submitted value leaked into output: %+v", got)
		}
		var result variableMutation
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil {
			t.Fatalf("stdout is not an acknowledgement: %v\n%s", err, got.stdout)
		}
		if result.Action != "created" || result.Key != "API_HOST" || result.Scope != "project" || result.ProjectID != "project-one" {
			t.Errorf("result = %+v", result)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.VariablesPath {
			t.Errorf("request = %s %s", req.Method, req.URL.Path)
		}
		assertCLIJSON(t, `{"key":"API_HOST","value":"`+submitted+`","projectId":"project-one"}`, f.lastBody())
	})

	t.Run("update to empty global value", func(t *testing.T) {
		f := variableFixture(t, "")
		f.status = http.StatusNoContent
		got := f.run("variable", "update", "var/id?x=1", "--key", "OPTIONAL", "--value", "")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		for _, want := range []string{"Updated:", "OPTIONAL", "var/id?x=1", "global"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("stdout missing %q: %s", want, got.stdout)
			}
		}
		req := f.lastRequest()
		if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+"/variables/var%2Fid%3Fx=1" {
			t.Errorf("request = %s %s, want escaped variable ID", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"key":"OPTIONAL","value":""}`, f.lastBody())
	})
}

func TestVariableWriteConflictDoesNotExposeValue(t *testing.T) {
	const submitted = "private-submitted-value"
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "create", args: []string{"variable", "create", "DUPLICATE", "--value", submitted}},
		{name: "update", args: []string{"variable", "update", "id", "--key", "DUPLICATE", "--value", submitted}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := variableFixture(t, "")
			f.status = http.StatusConflict
			got := f.run(tt.args...)
			if got.code != ExitError || got.stdout != "" {
				t.Errorf("result = %+v, want failure with empty stdout", got)
			}
			for _, want := range []string{"409", `"DUPLICATE"`, "unique", "variable list"} {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want %q", got.stderr, want)
				}
			}
			if strings.Contains(got.stderr, submitted) {
				t.Errorf("stderr leaked submitted value: %q", got.stderr)
			}
		})
	}
}

func TestVariableDeleteRequiresConfirmation(t *testing.T) {
	f := variableFixture(t, "")
	f.status = http.StatusNoContent

	before := f.requestCount()
	f.stdin = "no\n"
	got := f.run("variable", "delete", "variable-id")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") {
		t.Errorf("declined result = %+v", got)
	}
	if f.requestCount() != before {
		t.Error("declined delete reached the instance")
	}
	if !strings.Contains(got.stderr, "Workflows") {
		t.Errorf("prompt = %q, want consequence stated", got.stderr)
	}

	f.interactive = false
	got = f.run("variable", "delete", "variable-id")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("variable", "delete", "var/id?x=1", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	var result variableMutation
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Action != "deleted" || result.ID != "var/id?x=1" {
		t.Errorf("stdout = %q (error: %v)", got.stdout, err)
	}
	if req := f.lastRequest(); req.Method != http.MethodDelete || req.URL.EscapedPath() != n8n.BasePath+"/variables/var%2Fid%3Fx=1" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
}

func TestVariableValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "negative limit", args: []string{"variable", "list", "--limit", "-1"}, want: "negative"},
		{name: "invalid state", args: []string{"variable", "list", "--state", "set"}, want: "invalid"},
		{name: "padded project filter", args: []string{"variable", "list", "--project-id", " project"}, want: "whitespace"},
		{name: "unknown output", args: []string{"variable", "list", "--output", "yaml"}, want: "unknown output format"},
		{name: "blank create key", args: []string{"variable", "create", " ", "--value", "value"}, want: "variable key is required"},
		{name: "missing create value", args: []string{"variable", "create", "KEY"}, want: "--value is required"},
		{name: "padded write project", args: []string{"variable", "create", "KEY", "--value", "value", "--project-id", "project "}, want: "whitespace"},
		{name: "blank update ID", args: []string{"variable", "update", " ", "--key", "KEY", "--value", "value"}, want: "variable ID is required"},
		{name: "missing update key", args: []string{"variable", "update", "id", "--value", "value"}, want: "--key is required"},
		{name: "missing update value", args: []string{"variable", "update", "id", "--key", "KEY"}, want: "--value is required"},
		{name: "blank delete ID", args: []string{"variable", "delete", " ", "--yes"}, want: "variable ID is required"},
		{name: "missing create argument", args: []string{"variable", "create", "--value", "value"}, want: "accepts 1 arg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := variableFixture(t, `{"data":[],"nextCursor":null}`)
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want an error containing %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("invalid invocation reached the instance")
			}
		})
	}
}

func TestVariableScopeAndNotFoundErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		want   []string
	}{
		{name: "list unauthorized", status: http.StatusUnauthorized, args: []string{"variable", "list"}, want: []string{"401", "auth login"}},
		{name: "list forbidden", status: http.StatusForbidden, args: []string{"variable", "list"}, want: []string{"403", "discover --resource variable"}},
		{name: "update not found", status: http.StatusNotFound, args: []string{"variable", "update", "missing", "--key", "KEY", "--value", "private-value"}, want: []string{"404", "details redacted"}},
		{name: "delete not found", status: http.StatusNotFound, args: []string{"variable", "delete", "missing", "--yes"}, want: []string{"404"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := variableFixture(t, `{"data":[],"nextCursor":null}`)
			f.status = tt.status
			got := f.run(tt.args...)
			if got.code != ExitError || got.stdout != "" {
				t.Errorf("result = %+v, want failure with empty stdout", got)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want %q", got.stderr, want)
				}
			}
			if strings.Contains(got.stderr, "private-value") {
				t.Errorf("stderr leaked submitted value: %q", got.stderr)
			}
		})
	}
}
