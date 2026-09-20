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

const cliRoleMappingRule = `{
  "id":"rule/one?x=1",
  "expression":"groups contains \"admins\"",
  "role":"project:admin",
  "type":"project",
  "order":1,
  "projectIds":["project-one","project-two"],
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z"
}`

func roleMappingFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func writeRoleMappingInput(t *testing.T, f *fixture, name, body string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(f.dir), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write role-mapping input: %v", err)
	}
	return path
}

func TestRoleMappingListTextAndJSON(t *testing.T) {
	f := roleMappingFixture(t, `{"data":[`+cliRoleMappingRule+`],"nextCursor":"page two"}`)

	got := f.run("role-mapping", "list", "--type", "project", "--limit", "25", "--cursor", "start here")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Rules:", "TYPE", "ORDER", "rule/one?x=1", "project:admin", "project-one,project-two", "Next cursor:", "page two"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.RoleMappingRulesPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	query := req.URL.Query()
	if query.Get("type") != "project" || query.Get("limit") != "25" || query.Get("cursor") != "start here" {
		t.Errorf("query = %q, want type and pagination", req.URL.RawQuery)
	}

	got = f.run("role-mapping", "list", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.RoleMappingRule]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil {
		t.Fatalf("stdout is not a rule page: %v\n%s", err, got.stdout)
	}
	if len(page.Data) != 1 || page.Data[0].Order != 1 || page.Data[0].Type != n8n.RoleMappingRuleTypeProject {
		t.Errorf("page = %+v, want one project rule at order 1", page)
	}
	if query := f.lastRequest().URL.RawQuery; query != "" {
		t.Errorf("default list query = %q, want empty", query)
	}
}

func TestRoleMappingListAllFollowsEveryPage(t *testing.T) {
	f := roleMappingFixture(t, "")
	pages := []string{
		`{"data":[{"id":"1","type":"instance","role":"global:admin","order":0,"expression":"a"}],"nextCursor":"two"}`,
		`{"data":[{"id":"2","type":"instance","role":"global:member","order":1,"expression":"b"}],"nextCursor":null}`,
	}
	f.bodyFunc = func(i int) string { return pages[i] }

	got := f.run("role-mapping", "list", "--all", "--type", "instance", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.RoleMappingRule]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 2 {
		t.Fatalf("stdout = %q, want both pages (error: %v)", got.stdout, err)
	}
	if cursor := f.lastRequest().URL.Query().Get("cursor"); cursor != "two" {
		t.Errorf("second page cursor = %q, want %q", cursor, "two")
	}
}

func TestRoleMappingListEmptyAdvisesCreation(t *testing.T) {
	f := roleMappingFixture(t, `{"data":[],"nextCursor":null}`)
	got := f.run("role-mapping", "list")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "role-mapping create") {
		t.Errorf("result = %+v, want an empty-list hint on stderr", got)
	}
}

func TestRoleMappingListRejectsBadFlagsBeforeTransport(t *testing.T) {
	f := roleMappingFixture(t, `{"data":[],"nextCursor":null}`)
	before := f.requestCount()

	for _, args := range [][]string{
		{"role-mapping", "list", "--type", "global"},
		{"role-mapping", "list", "--output", "yaml"},
		{"role-mapping", "list", "--limit", "-1"},
	} {
		got := f.run(args...)
		if got.code != ExitError {
			t.Errorf("%v exit = %d, want %d", args, got.code, ExitError)
		}
	}
	if f.requestCount() != before {
		t.Errorf("requests = %d, want validation before transport", f.requestCount()-before)
	}
}

func TestRoleMappingCreateFromFileAndStdin(t *testing.T) {
	f := roleMappingFixture(t, cliRoleMappingRule)
	path := writeRoleMappingInput(t, f, "rule.json",
		`{"expression":"dept == \"ops\"","role":"project:admin","type":"project","order":0,"projectIds":["project-one"]}`)

	got := f.run("role-mapping", "create", "--input", path)
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Created:", "rule/one?x=1", "Order:", "Projects:", "project-one,project-two"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.RoleMappingRulesPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body := strings.TrimSpace(f.lastBody()); !strings.Contains(body, `"order":0`) || !strings.Contains(body, `"projectIds":["project-one"]`) {
		t.Errorf("body = %s, want the submitted order and projects", body)
	}

	f.stdin = `{"expression":"true","role":"global:member","type":"instance"}`
	got = f.run("role-mapping", "create", "--input", "-", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("stdin exit = %d, stderr: %s", got.code, got.stderr)
	}
	var rule n8n.RoleMappingRule
	if err := json.Unmarshal([]byte(got.stdout), &rule); err != nil || rule.ID != "rule/one?x=1" {
		t.Errorf("stdout = %q, want the created rule (error: %v)", got.stdout, err)
	}
	if body := strings.TrimSpace(f.lastBody()); strings.Contains(body, "order") || strings.Contains(body, "projectIds") {
		t.Errorf("body = %s, want order and projectIds omitted", body)
	}
}

func TestRoleMappingCreateRejectsBadInputBeforeTransport(t *testing.T) {
	tests := map[string]struct {
		stdin string
		args  []string
		want  string
	}{
		"missing input":    {args: []string{"role-mapping", "create"}, want: "--input is required"},
		"unknown field":    {stdin: `{"expression":"a","role":"r","type":"instance","nope":1}`, want: "unknown field"},
		"two documents":    {stdin: `{"expression":"a","role":"r","type":"instance"} {"expression":"b"}`, want: "multiple documents"},
		"unknown type":     {stdin: `{"expression":"a","role":"r","type":"global"}`, want: "invalid"},
		"project on inst.": {stdin: `{"expression":"a","role":"r","type":"instance","projectIds":["p"]}`, want: "project IDs apply only"},
		"empty expression": {stdin: `{"expression":"  ","role":"r","type":"instance"}`, want: "expression is required"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			f := roleMappingFixture(t, cliRoleMappingRule)
			f.stdin = tt.stdin
			before := f.requestCount()
			args := tt.args
			if args == nil {
				args = []string{"role-mapping", "create", "--input", "-"}
			}
			got := f.run(args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want an error containing %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Errorf("requests = %d, want validation before transport", f.requestCount()-before)
			}
		})
	}
}

func TestRoleMappingUpdateSendsPatchWithPresentFieldsOnly(t *testing.T) {
	f := roleMappingFixture(t, cliRoleMappingRule)
	f.stdin = `{"role":"global:member"}`

	got := f.run("role-mapping", "update", "rule/one?x=1", "--input", "-")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, "Updated:") {
		t.Errorf("stdout = %q, want an update acknowledgement", got.stdout)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPatch || req.URL.EscapedPath() != n8n.BasePath+"/role-mapping-rules/rule%2Fone%3Fx=1" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	if body := strings.TrimSpace(f.lastBody()); body != `{"role":"global:member"}` {
		t.Errorf("body = %s, want only the role", body)
	}

	f.stdin = `{"projectIds":[]}`
	if got := f.run("role-mapping", "update", "rule-1", "--input", "-"); got.code != ExitSuccess {
		t.Fatalf("clearing projects exit = %d, stderr: %s", got.code, got.stderr)
	}
	if body := strings.TrimSpace(f.lastBody()); body != `{"projectIds":[]}` {
		t.Errorf("body = %s, want an explicit empty project array", body)
	}
}

func TestRoleMappingUpdateRejectsEmptyAndReorderingDocuments(t *testing.T) {
	for name, input := range map[string]string{
		"empty document": `{}`,
		"type change":    `{"type":"instance"}`,
		"order change":   `{"order":0}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := roleMappingFixture(t, cliRoleMappingRule)
			f.stdin = input
			before := f.requestCount()
			got := f.run("role-mapping", "update", "rule-1", "--input", "-")
			if got.code != ExitError {
				t.Errorf("exit = %d, want %d (stderr: %s)", got.code, ExitError, got.stderr)
			}
			if f.requestCount() != before {
				t.Errorf("requests = %d, want validation before transport", f.requestCount()-before)
			}
		})
	}
}

func TestRoleMappingMoveRequiresTargetIndex(t *testing.T) {
	f := roleMappingFixture(t, cliRoleMappingRule)
	before := f.requestCount()

	got := f.run("role-mapping", "move", "rule-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "--target-index is required") {
		t.Errorf("result = %+v, want the missing-flag error", got)
	}
	got = f.run("role-mapping", "move", "rule-1", "--target-index", "-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "must not be negative") {
		t.Errorf("negative result = %+v, want a validation error", got)
	}
	if f.requestCount() != before {
		t.Errorf("requests = %d, want validation before transport", f.requestCount()-before)
	}

	got = f.run("role-mapping", "move", "rule/one?x=1", "--target-index", "0", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+"/role-mapping-rules/rule%2Fone%3Fx=1/move" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	if body := strings.TrimSpace(f.lastBody()); body != `{"targetIndex":0}` {
		t.Errorf("body = %s, want the target index", body)
	}
	var rule n8n.RoleMappingRule
	if err := json.Unmarshal([]byte(got.stdout), &rule); err != nil || rule.Order != 1 {
		t.Errorf("stdout = %q, want the moved rule (error: %v)", got.stdout, err)
	}
}

func TestRoleMappingDeleteRequiresConfirmation(t *testing.T) {
	f := roleMappingFixture(t, cliRoleMappingRule)
	before := f.requestCount()
	f.stdin = "no\n"

	got := f.run("role-mapping", "delete", "rule-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}
	if !strings.Contains(got.stderr, "next sign-in") {
		t.Errorf("prompt = %q, want the access consequence", got.stderr)
	}

	f.interactive = false
	got = f.run("role-mapping", "delete", "rule-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v, requests = %d want %d", got, f.requestCount(), before)
	}

	got = f.run("role-mapping", "delete", "rule/one?x=1", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodDelete || req.URL.EscapedPath() != n8n.BasePath+"/role-mapping-rules/rule%2Fone%3Fx=1" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	var rule n8n.RoleMappingRule
	if err := json.Unmarshal([]byte(got.stdout), &rule); err != nil || rule.ID != "rule/one?x=1" {
		t.Errorf("stdout = %q, want the deleted rule (error: %v)", got.stdout, err)
	}
}

func TestRoleMappingBadRequestExplainsTheLikelyCause(t *testing.T) {
	f := roleMappingFixture(t, cliRoleMappingRule)
	f.status = http.StatusBadRequest
	f.stdin = `{"expression":"a","role":"global:nope","type":"instance"}`

	got := f.run("role-mapping", "create", "--input", "-")
	if got.code != ExitError {
		t.Fatalf("exit = %d, want %d", got.code, ExitError)
	}
	for _, want := range []string{"n8n role list", "n8n project list", "expression"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, got.stderr)
		}
	}
}

func TestRoleMappingDenialsAreReported(t *testing.T) {
	f := roleMappingFixture(t, cliRoleMappingRule)
	f.status = http.StatusUnauthorized
	got := f.run("role-mapping", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "n8n auth login") {
		t.Errorf("401 result = %+v, want the credential hint", got)
	}

	f.status = http.StatusForbidden
	got = f.run("role-mapping", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "n8n discover --resource rolemappingrule") || !strings.Contains(got.stderr, "roleMappingRule:list") {
		t.Errorf("403 result = %+v, want the discover hint for this resource", got)
	}
}
