package cli

import (
	"net/http"
	"strings"
	"testing"
)

// n8n 2.47 shapes: blocked carries check, preview-changed joins the statuses,
// project apply takes expectedPreviewRevision, scopes moved to promotions:*,
// and environments list their missing projects.

const cliPreviewRevision = "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"

const cliCheckBlockedBody = `{
  "environmentId":"` + cliEnvironmentID + `",
  "git":{"commitSha":"` + cliSelectionSHA + `","branchName":"main"},
  "status":"blocked",
  "check":{
    "missingProjects":[{"id":"p-9","name":"Ops"}],
    "missingBindings":[],
    "accessRequirements":[],
    "conflicts":[
      {"kind":"workflow","code":"lineage-conflict","reason":"history diverged","consumers":[{"project":{"id":"p1","name":"Sales"},"workflows":[{"id":"w1","name":"Lead"}]}]},
      {"kind":"project","code":"project-not-on-branch","project":{"id":"p2","name":"Ops"},"reason":"absent from the branch"},
      {"kind":"folder","code":"moved","id":"f1","name":"Inbox","project":{"id":"p1","name":"Sales"},"projectExists":false,"instanceProject":{"id":"p3","name":"Support"}},
      {"kind":"data-table","code":"destructive-change","id":"dt1","name":"Leads","changes":[{"kind":"remove-column","column":"score","destructive":true}]}
    ],
    "warnings":[]
  }
}`

const cliPreviewChangedBody = `{
  "environmentId":"` + cliEnvironmentID + `",
  "git":{"commitSha":"` + cliSelectionSHA + `","branchName":"main"},
  "status":"preview-changed"
}`

func TestPromotionApplyRendersCheck(t *testing.T) {
	for _, args := range [][]string{
		{"promotion", "environment", "apply", cliEnvironmentID, "--yes"},
		{"promotion", "project", "apply", "p", "--workflow-id", "w", "--yes"},
	} {
		f := promotionFixture(t, cliCheckBlockedBody)
		got := f.run(args...)
		if got.code != ExitPromotionBlocked {
			t.Fatalf("%v: exit = %d, stderr: %s", args, got.code, got.stderr)
		}
		for _, want := range []string{
			"MISSING PROJECTS (1)", "p-9",
			"CONFLICTS (4)", "lineage-conflict", "history diverged", "1 workflow(s) in 1 project(s)",
			"project-not-on-branch", "absent from the branch", "project p2",
			"moved", "branch project Sales (p1), not on this instance, instance project Support (p3)",
			"remove-column score",
		} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("%v: stdout missing %q:\n%s", args, want, got.stdout)
			}
		}
		if strings.Contains(got.stdout, "no items reported") {
			t.Errorf("%v: stdout = %s, want the check items", args, got.stdout)
		}
		for _, want := range []string{"pre-import check", "explicit confirmation", "apply-continue", "environmentId=" + cliEnvironmentID} {
			if !strings.Contains(got.stderr, want) {
				t.Errorf("%v: stderr missing %q:\n%s", args, want, got.stderr)
			}
		}
		got = f.run(append(args, "--output", "json")...)
		if got.code != ExitPromotionBlocked || !sameJSON(t, got.stdout, cliCheckBlockedBody) {
			t.Errorf("%v: json = %+v", args, got)
		}
	}

	// An empty check says so rather than printing nothing.
	f := promotionFixture(t, strings.Replace(cliEnvBlockedBody, `"preflight"`, `"check"`, 1))
	f.body = strings.Replace(f.body, `[{"id":"p-9","name":"Ops"}]`, `[]`, 1)
	if got := f.run("promotion", "environment", "apply", cliEnvironmentID, "--yes"); got.code != ExitPromotionBlocked || !strings.Contains(got.stdout, "Check:") {
		t.Errorf("empty check = %+v", got)
	}
}

func TestPromotionApplyPreviewChanged(t *testing.T) {
	for _, tc := range []struct {
		args []string
		hint string
	}{
		{[]string{"promotion", "environment", "apply", cliEnvironmentID}, "n8n promotion environment apply " + cliEnvironmentID},
		{append([]string{"promotion", "environment", "apply-continue", cliEnvironmentID}, envExpectedFlags()...), "n8n promotion environment apply " + cliEnvironmentID},
		{[]string{"promotion", "project", "apply", "p", "--workflow-id", "w"}, "n8n promotion project apply p"},
		{append([]string{"promotion", "project", "apply-continue", "p", "--workflow-id", "w"}, envExpectedFlags()...), "n8n promotion project apply p"},
	} {
		f := promotionFixture(t, cliPreviewChangedBody)
		got := f.run(append(tc.args, "--yes", "--output", "json")...)
		if got.code != ExitPromotionSourceChanged || !sameJSON(t, got.stdout, cliPreviewChangedBody) {
			t.Errorf("%v: result = %+v, want exit %d", tc.args, got, ExitPromotionSourceChanged)
		}
		for _, want := range []string{"Apply preview was reviewed", tc.hint, "nothing was applied"} {
			if !strings.Contains(got.stderr, want) {
				t.Errorf("%v: stderr missing %q:\n%s", tc.args, want, got.stderr)
			}
		}
		if got = f.run(append(tc.args, "--yes")...); got.code != ExitPromotionSourceChanged || !strings.Contains(got.stdout, "preview-changed") {
			t.Errorf("%v text = %+v", tc.args, got)
		}
	}
	if code := (&promotionApplyStoppedError{status: "preview-changed"}).exitCode(); code != ExitPromotionSourceChanged {
		t.Errorf("preview-changed exit = %d, want %d", code, ExitPromotionSourceChanged)
	}
}

func TestPromotionProjectApplyExpectedPreviewRevision(t *testing.T) {
	f := promotionFixture(t, cliEnvAppliedBody)
	for _, tc := range []struct {
		args       []string
		path, body string
	}{
		{[]string{"promotion", "project", "apply", "p", "--workflow-id", "w", "--expected-preview-revision", cliPreviewRevision},
			"/promotions/projects/p/apply", `{"workflowIds":["w"],"expectedPreviewRevision":"` + cliPreviewRevision + `"}`},
		{append([]string{"promotion", "project", "apply-continue", "p", "--workflow-id", "w", "--expected-preview-revision", cliPreviewRevision}, envExpectedFlags()...),
			"/promotions/projects/p/apply/continue", `{"workflowIds":["w"],"expectedSource":` + cliEnvSource + `,"expectedPreviewRevision":"` + cliPreviewRevision + `"}`},
		{[]string{"promotion", "project", "apply", "p", "--workflow-id", "w"},
			"/promotions/projects/p/apply", `{"workflowIds":["w"]}`},
	} {
		if got := f.run(append(tc.args, "--yes")...); got.code != ExitSuccess {
			t.Fatalf("%v: %+v", tc.args, got)
		}
		req := f.lastRequest()
		if req.URL.EscapedPath() != "/api/v1"+tc.path || f.lastBody() != tc.body {
			t.Errorf("%v: request = %s %s", tc.args, req.URL.EscapedPath(), f.lastBody())
		}
	}

	input := writePromotionInput(t, f, "revision.json", `{"workflowIds":["w"],"expectedPreviewRevision":"`+cliPreviewRevision+`"}`)
	if got := f.run("promotion", "project", "apply", "p", "--input", input, "--yes"); got.code != ExitSuccess || f.lastBody() != `{"workflowIds":["w"],"expectedPreviewRevision":"`+cliPreviewRevision+`"}` {
		t.Errorf("input = %+v body=%s", got, f.lastBody())
	}

	before := f.requestCount()
	for _, args := range [][]string{
		{"promotion", "project", "apply", "p", "--workflow-id", "w", "--expected-preview-revision", "ABC", "--yes"},
		{"promotion", "project", "apply", "p", "--input", writePromotionInput(t, f, "bad.json", `{"workflowIds":["w"],"expectedPreviewRevision":"nope"}`), "--yes"},
		{"promotion", "project", "apply", "p", "--input", input, "--expected-preview-revision", cliPreviewRevision, "--yes"},
	} {
		if got := f.run(args...); got.code == ExitSuccess {
			t.Errorf("%v accepted: %+v", args, got)
		}
	}
	if f.requestCount() != before {
		t.Errorf("invalid revisions reached the instance")
	}
	for _, args := range [][]string{
		{"promotion", "environment", "apply", cliEnvironmentID, "--expected-preview-revision", cliPreviewRevision, "--yes"},
		{"promotion", "project", "promote", "p", "--workflow-id", "w", "--expected-preview-revision", cliPreviewRevision, "--yes"},
	} {
		if got := f.run(args...); got.code == ExitSuccess || !strings.Contains(got.stderr, "unknown flag") {
			t.Errorf("%v = %+v, want unknown flag", args, got)
		}
	}
}

func TestPromotionScopesNameBothGenerations(t *testing.T) {
	for _, tc := range []struct {
		args          []string
		current, prev string
	}{
		{[]string{"promotion", "environment", "apply", cliEnvironmentID, "--yes"}, "promotions:apply", "gitConnection:pull"},
		{[]string{"promotion", "environment", "disconnect", cliEnvironmentID, "--yes"}, "promotions:connect", "gitConnection:clone"},
		{[]string{"promotion", "provider", "list"}, "promotions:list", "gitConnection:list"},
		{[]string{"promotion", "project", "promote", "p", "--workflow-id", "w", "--yes"}, "promotions:promote", "gitConnection:push"},
	} {
		f := promotionFixture(t, `{"message":"Forbidden"}`)
		f.status = http.StatusForbidden
		got := f.run(tc.args...)
		for _, want := range []string{tc.current, tc.prev, "either one", "n8n 2.47 and later check " + tc.current} {
			if got.code != ExitError || !strings.Contains(got.stderr, want) {
				t.Errorf("%v: stderr missing %q:\n%s", tc.args, want, got.stderr)
			}
		}
	}
	// Connection routes predate the rename and keep one scope.
	f := promotionFixture(t, `{"message":"Forbidden"}`)
	f.status = http.StatusForbidden
	got := f.run("promotion", "connection", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "gitConnection:list") || strings.Contains(got.stderr, "promotions:") {
		t.Errorf("connection 403 = %+v", got)
	}
}

const cliMissingProjectsBody = `{
  "environmentId":"` + cliEnvironmentID + `",
  "git":{"commitSha":"` + cliSelectionSHA + `","branchName":"main"},
  "data":[{"id":"p-9","name":"Ops","icon":{"type":"emoji","value":"📣"},"description":"Operations\nteam","customTelemetryTags":[{"key":"team","value":"ops"}]}]
}`

func TestPromotionEnvironmentMissingProjects(t *testing.T) {
	f := promotionFixture(t, cliMissingProjectsBody)
	got := f.run("promotion", "environment", "missing-projects", cliEnvironmentID)
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != cliEnvironmentPath+"/missing-projects" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	for _, want := range []string{"Missing projects:", "p-9", "Ops", "Operations team", cliSelectionSHA, "main"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if !strings.Contains(got.stderr, "n8n project create NAME --id PROJECT_ID") {
		t.Errorf("stderr = %s, want the create hint", got.stderr)
	}

	got = f.run("promotion", "environment", "missing-projects", cliEnvironmentID, "--output", "json")
	if got.code != ExitSuccess || !sameJSON(t, got.stdout, cliMissingProjectsBody) {
		t.Errorf("json = %+v", got)
	}

	f.body = `{"environmentId":"` + cliEnvironmentID + `","git":null,"data":[]}`
	got = f.run("promotion", "environment", "missing-projects", cliEnvironmentID)
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "no package yet") || !strings.Contains(got.stderr, "no setup is needed") {
		t.Errorf("empty = %+v", got)
	}

	before := f.requestCount()
	if got = f.run("promotion", "environment", "missing-projects", "conn-1"); got.code == ExitSuccess || f.requestCount() != before {
		t.Errorf("invalid ID = %+v", got)
	}

	answer := func(status int, body string) {
		f.route = func(*http.Request, int) (int, string) { return status, body }
	}
	answer(http.StatusNotFound, `{"message":"Not Found"}`)
	got = f.run("promotion", "environment", "missing-projects", cliEnvironmentID)
	if got.code != ExitError || !strings.Contains(got.stderr, "predates n8n 2.47") {
		t.Errorf("route 404 = %+v", got)
	}
	answer(http.StatusNotFound, `{"message":"Promotion environment not found"}`)
	got = f.run("promotion", "environment", "missing-projects", cliEnvironmentID)
	if got.code != ExitError || !strings.Contains(got.stderr, "has no promotion environment") {
		t.Errorf("environment 404 = %+v", got)
	}
	answer(http.StatusForbidden, `{"message":"Forbidden"}`)
	got = f.run("promotion", "environment", "missing-projects", cliEnvironmentID)
	if got.code != ExitError || !strings.Contains(got.stderr, "promotions:read") || strings.Contains(got.stderr, "gitConnection") {
		t.Errorf("403 = %+v", got)
	}
}
