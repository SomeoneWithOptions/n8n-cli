package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliPreview249Revision = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

// cliChanges249 is an n8n 2.49 Apply preview with resources, a dependency,
// a moved workflow, a blocking conflict, and the preview revision.
const cliChanges249 = `{
  "commitSha":"` + cliSelectionSHA + `",
  "source":{"environmentId":"env-1","branchName":"main"},
  "resources":[
    {"key":"project:p1","type":"project","id":"p1","name":"Ops","parentKey":null,"change":"unchanged","selectable":false,"revision":{"before":"r","after":"r"}},
    {"key":"folder:f1","type":"folder","id":"f1","name":"Billing","previousName":"Bills","parentKey":"project:p1","change":"modified","selectable":true,"revision":{"before":"a","after":"b"}},
    {"key":"workflow:w1","type":"workflow","id":"w1","name":"Invoice","parentKey":"folder:f1","change":"added","selectable":true,"movedFromProject":{"id":"p2","name":"Sales"},"revision":{"before":null,"after":"c"}},
    {"key":"credential:c1","type":"credential","id":"c1","name":"Stripe","parentKey":null,"change":"modified","selectable":false,"revision":{"before":"d","after":"e"}}
  ],
  "dependencies":[{"workflowKey":"workflow:w1","resourceKey":"credential:c1","included":true}],
  "check":{"missingProjects":[],"missingBindings":[],"accessRequirements":[],"warnings":[],
    "conflicts":[{"kind":"tag","code":"tag-conflict","id":"t1","name":"prod","reason":"name-collision","consumers":[{"project":{"id":"p1","name":"Ops"},"workflows":[{"id":"w1","name":"Invoice"}]}]}],
    "movedWorkflows":[{"id":"w9","name":"Moved","project":{"id":"p2","name":"Sales"},"projectExists":false,"instanceProject":{"id":"p1"}}],
    "movedFolders":[]},
  "revision":"` + cliPreview249Revision + `"
}`

func TestPromotionChanges249(t *testing.T) {
	f := promotionFixture(t, cliChanges249)
	got := f.run("promotion", "project", "changes", "p1", "apply")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if q := f.lastRequest().URL.RawQuery; q != "" {
		t.Errorf("query = %q, want no filters by default", q)
	}
	for _, want := range []string{
		"Environment:", "env-1", "Preview revision:", cliPreview249Revision,
		"KEY", "SELECTABLE", "folder:f1", "Bills -> Billing", "workflow:w1", "added",
		"CONFLICTS (1)", "tag-conflict", "name-collision",
		"MOVED WORKFLOWS (1)", "Sales (p2), not on this instance",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	for _, want := range []string{"moved here from project Sales (p2)", "would be blocked"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, got.stderr)
		}
	}
	got = f.run("promotion", "project", "changes", "p1", "apply", "--output", "json")
	if got.code != ExitSuccess || !sameJSON(t, got.stdout, cliChanges249) {
		t.Errorf("json = %+v", got)
	}

	f.body = `{"commitSha":null,"source":{"environmentId":"env-1","branchName":"main"},"resources":[],"dependencies":[],"blocker":{"code":"reviewOpen","reviewId":"42"}}`
	got = f.run("promotion", "project", "changes", "p1", "promote")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "reviewOpen (review 42)") || !strings.Contains(got.stderr, "open review (42)") {
		t.Errorf("blocker = %+v", got)
	}

	// The pre-2.49 filters are deprecated: the notice goes to stderr, and a
	// 2.49 rejection says to drop them.
	f.respond(http.StatusBadRequest, `{"message":"Unknown query parameter 'search'"}`)
	got = f.run("promotion", "project", "changes", "p1", "apply", "--search", "x")
	if got.code == ExitSuccess || !strings.Contains(got.stderr, "dropped --search") || !strings.Contains(got.stderr, "has been deprecated") || strings.Contains(got.stdout, "deprecated") {
		t.Errorf("filters on 2.49 = %+v", got)
	}
}

func TestPromotionSelectionFolders(t *testing.T) {
	f := promotionFixture(t, cliPromotionAppliedBody)
	got := f.run("promotion", "project", "apply", "p1", "--folder-id", "f1", "--workflow-id", "w1", "--yes")
	if got.code != ExitSuccess || f.lastBody() != `{"workflowIds":["w1"],"folderIds":["f1"]}` {
		t.Fatalf("apply = %+v body %s", got, f.lastBody())
	}

	// No selection applies only the project's details; the prompt says so.
	f.stdin = "no\n"
	before := f.requestCount()
	got = f.run("promotion", "project", "apply", "p1")
	if got.code == ExitSuccess || !strings.Contains(got.stderr, "Apply only the details") || f.requestCount() != before {
		t.Errorf("empty prompt = %+v", got)
	}
	got = f.run("promotion", "project", "apply", "p1", "--yes")
	if got.code != ExitSuccess || f.lastBody() != `{}` {
		t.Errorf("empty apply = %+v body %s", got, f.lastBody())
	}
	f.body = cliPromotionPromoteBody
	f.stdin = "no\n"
	if got = f.run("promotion", "project", "promote", "p1"); !strings.Contains(got.stderr, "Promote only the details") {
		t.Errorf("empty promote prompt = %+v", got)
	}
	got = f.run("promotion", "project", "promote", "p1", "--folder-id", "f1", "--yes")
	if got.code != ExitSuccess || f.lastBody() != `{"folderIds":["f1"]}` {
		t.Errorf("promote = %+v body %s", got, f.lastBody())
	}

	// Instances before n8n 2.49 reject folders and empty selections.
	f.respond(http.StatusBadRequest, `{"code":"unrecognized_keys","keys":["folderIds"],"path":[],"message":"Unrecognized key(s) in object: 'folderIds'"}`)
	if got = f.run("promotion", "project", "promote", "p1", "--folder-id", "f1", "--yes"); got.code == ExitSuccess || !strings.Contains(got.stderr, "--folder-id needs n8n 2.49") {
		t.Errorf("old folders = %+v", got)
	}
	f.respond(http.StatusBadRequest, `{"code":"invalid_type","path":["workflowIds"],"message":"Required"}`)
	if got = f.run("promotion", "project", "apply", "p1", "--yes"); got.code == ExitSuccess || !strings.Contains(got.stderr, "need at least one --workflow-id") {
		t.Errorf("old empty = %+v", got)
	}
}

func TestPromotionLocked(t *testing.T) {
	f := promotionFixture(t, "")
	f.respond(http.StatusLocked, `{"message":"The resource is locked by another operation."}`)
	for _, args := range [][]string{
		{"promotion", "project", "apply", "p1", "--workflow-id", "w", "--yes"},
		{"promotion", "environment", "promote", cliEnvironmentID, "--yes"},
	} {
		got := f.run(args...)
		if got.code == ExitSuccess || !strings.Contains(got.stderr, "(423)") || !strings.Contains(got.stderr, "wait for it to finish and retry") {
			t.Errorf("%v = %+v", args, got)
		}
	}
}

func TestPromotionExpectedFromPinsPreviewRevision(t *testing.T) {
	f := promotionFixture(t, cliPromotionAppliedBody)
	path := writePromotionInput(t, f, "changes.json", cliChanges249)
	got := f.run("promotion", "project", "apply", "p1", "--workflow-id", "w1", "--expected-from", path, "--yes")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "Using the preview revision") {
		t.Fatalf("apply = %+v", got)
	}
	var sent struct {
		ExpectedSource          map[string]string `json:"expectedSource"`
		ExpectedPreviewRevision string            `json:"expectedPreviewRevision"`
	}
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil || sent.ExpectedPreviewRevision != cliPreview249Revision || sent.ExpectedSource["environmentId"] != "env-1" {
		t.Errorf("body = %s", f.lastBody())
	}
	other := strings.Repeat("b", 64)
	got = f.run("promotion", "project", "apply", "p1", "--workflow-id", "w1", "--expected-from", path, "--expected-preview-revision", other, "--yes")
	if got.code != ExitSuccess || !strings.Contains(f.lastBody(), other) || strings.Contains(got.stderr, "Using the preview revision") {
		t.Errorf("explicit revision = %+v body %s", got, f.lastBody())
	}
}

const cliChangeDetails = `{
  "details":[
    {"key":"workflow:w1","type":"workflow","before":null,"after":{"id":"w1","name":"Invoice"},"revision":{"before":null,"after":"c2"}},
    {"key":"folder:f1","type":"folder","before":{"id":"f1","name":"Bills","parentFolderId":null},"after":{"id":"f1","name":"Billing","parentFolderId":null},"revision":{"before":"a","after":"b"}},
    {"key":"credential:c1","type":"credential","before":{"id":"c1","name":"Stripe"},"after":{"id":"c1","name":"Stripe"},"revision":{"before":"d","after":"e"}}
  ],
  "missingKeys":[]
}`

func TestPromotionChangeDetails(t *testing.T) {
	f := promotionFixture(t, cliChangeDetails)
	path := writePromotionInput(t, f, "changes.json", cliChanges249)
	got := f.run("promotion", "project", "change-details", "p1", "apply", "--from", path)
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/promotions/projects/p1/changes/apply/details" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	want := `{"commitSha":"` + cliSelectionSHA + `","keys":["folder:f1","workflow:w1","credential:c1"]}`
	if f.lastBody() != want {
		t.Errorf("body = %s, want the changed keys of the preview", f.lastBody())
	}
	for _, want := range []string{"KEY", "workflow:w1", "Invoice", "added", "folder:f1", "Billing", "modified"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	// The workflow revision differs from the preview: it changed since.
	if !strings.Contains(got.stderr, "workflow:w1 changed after the preview") || strings.Contains(got.stderr, "folder:f1 changed") {
		t.Errorf("stderr = %s", got.stderr)
	}

	got = f.run("promotion", "project", "change-details", "p1", "promote", "--commit-sha", cliSelectionSHA, "--key", "folder:f1", "--output", "json")
	if got.code != ExitSuccess || !sameJSON(t, got.stdout, cliChangeDetails) {
		t.Errorf("json = %+v", got)
	}
	if f.lastBody() != `{"commitSha":"`+cliSelectionSHA+`","keys":["folder:f1"]}` {
		t.Errorf("explicit body = %s", f.lastBody())
	}

	f.body = `{"details":[{"key":"variable:v1","type":"variable","before":{"name":"TOKEN","type":"string"},"after":{"name":"TOKEN","type":"string"},"revision":{"before":"x","after":"y"},"valueHidden":true}],"missingKeys":["tag:t9"]}`
	got = f.run("promotion", "project", "change-details", "p1", "apply", "--commit-sha", cliSelectionSHA, "--key", "variable:v1", "--key", "tag:t9")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "values are hidden") || !strings.Contains(got.stderr, "Neither side holds tag:t9") {
		t.Errorf("hidden/missing = %+v", got)
	}

	before := f.requestCount()
	legacy := writePromotionInput(t, f, "legacy.json", cliSelectionChanges)
	for _, args := range [][]string{
		{"promotion", "project", "change-details", "p1", "apply"},
		{"promotion", "project", "change-details", "p1", "apply", "--commit-sha", cliSelectionSHA},
		{"promotion", "project", "change-details", "p1", "apply", "--commit-sha", "abc", "--key", "k"},
		{"promotion", "project", "change-details", "p1", "apply", "--from", path, "--commit-sha", cliSelectionSHA},
		{"promotion", "project", "change-details", "p1", "apply", "--from", legacy},
		{"promotion", "project", "change-details", "p1", "pull", "--commit-sha", cliSelectionSHA, "--key", "k"},
	} {
		if got := f.run(args...); got.code == ExitSuccess || f.requestCount() != before {
			t.Errorf("%v accepted: %+v", args, got)
		}
	}

	f.respond(http.StatusNotFound, `{"message":"Cannot POST /api/v1/promotions/projects/p1/changes/apply/details"}`)
	if got = f.run("promotion", "project", "change-details", "p1", "apply", "--from", path); got.code == ExitSuccess || !strings.Contains(got.stderr, "n8n 2.49 or later") {
		t.Errorf("404 = %+v", got)
	}
	f.respond(http.StatusForbidden, `{"message":"Forbidden"}`)
	if got = f.run("promotion", "project", "change-details", "p1", "promote", "--from", path); got.code == ExitSuccess || !strings.Contains(got.stderr, "promotions:promote") {
		t.Errorf("403 = %+v", got)
	}
}

// respond answers every request with status and body. The fixture replaces
// non-200 bodies unless a route answers, so error bodies go through route.
func (f *fixture) respond(status int, body string) {
	f.route = func(*http.Request, int) (int, string) { return status, body }
}
