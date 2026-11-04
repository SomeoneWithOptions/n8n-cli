package cli

import (
	"net/http"
	"strings"
	"testing"
)

// n8n 2.46 shapes: environmentId replaces connectionId and configId, folders
// gain updated/moved/renamed, workflows gain moved, credentials gain updated,
// and promote can add review and warnings.

const cliEnvSource = `{"environmentId":"` + cliEnvironmentID + `","branchName":"main","commitSha":"` + cliSelectionSHA + `"}`

const cliEnvAppliedBody = `{
  "environmentId":"` + cliEnvironmentID + `",
  "git":{"commitSha":"` + cliSelectionSHA + `","branchName":"main"},
  "status":"applied",
  "counts":{
    "projects":{"created":0,"updated":1,"skipped":0,"deleted":0},
    "folders":{"created":1,"updated":3,"skipped":0,"removed":0,"moved":2,"renamed":1},
    "workflows":{"created":0,"updated":4,"skipped":0,"archived":0,"deleted":0,"moved":2,"publishing":{"published":0,"unpublished":0,"unchanged":4,"blocked":0,"failed":0}},
    "credentials":{"matched":1,"stubbed":0,"updated":5},
    "dataTables":{"matched":0,"created":0,"updated":0},
    "variables":{"matched":0,"created":0,"updated":0,"stubbed":0,"missing":0},
    "tags":{"matched":0,"created":0,"renamed":0,"reconciled":0,"skipped":0}
  },
  "warnings":[]
}`

const cliEnvBlockedBody = `{
  "environmentId":"` + cliEnvironmentID + `",
  "git":{"commitSha":"` + cliSelectionSHA + `","branchName":"main"},
  "status":"blocked",
  "preflight":{"missingProjects":[{"id":"p-9","name":"Ops"}],"missingBindings":[],"accessRequirements":[],"conflicts":[],"warnings":[]}
}`

const cliEnvSourceChangedBody = `{
  "environmentId":"` + cliEnvironmentID + `",
  "git":{"commitSha":"` + cliSelectionSHA + `","branchName":"main"},
  "status":"source-changed"
}`

const cliEnvPromoteBody = `{
  "environmentId":"` + cliEnvironmentID + `",
  "counts":{"workflows":2,"folders":1,"credentials":0,"dataTables":0,"variables":0,"tags":3},
  "git":{"commitSha":"deadbeef","branchName":"n8n/promotion-1"},
  "review":{"reviewId":"mr-1","number":42,"webUrl":"https://gitlab.example.com/org/repo/-/merge_requests/42"},
  "warnings":[{"message":"Branch protection blocks merging","linkUrl":"https://gitlab.example.com/help"},{"message":"No reviewers"}]
}`

var cliEnvChanges = strings.Replace(cliSelectionChanges, `"configId":"cfg"`, `"environmentId":"`+cliEnvironmentID+`"`, 1)

func envExpectedFlags() []string {
	return []string{"--expected-environment-id", cliEnvironmentID, "--expected-branch", "main", "--expected-commit", cliSelectionSHA}
}

func TestPromotionEnvironmentSyncCommands(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		args                 []string
		path, body, response string
	}{
		{"apply", []string{"promotion", "environment", "apply", cliEnvironmentID}, "/apply", `{}`, cliEnvAppliedBody},
		{"apply with source", append([]string{"promotion", "environment", "apply", cliEnvironmentID}, envExpectedFlags()...), "/apply", `{"expectedSource":` + cliEnvSource + `}`, cliEnvAppliedBody},
		{"apply-continue", append([]string{"promotion", "environment", "apply-continue", cliEnvironmentID, "--confirm-destructive-changes"}, envExpectedFlags()...), "/apply/continue", `{"expectedSource":` + cliEnvSource + `,"confirmDestructiveChanges":true}`, cliEnvAppliedBody},
		{"promote", []string{"promotion", "environment", "promote", cliEnvironmentID}, "/promote", `{}`, cliEnvPromoteBody},
		{"promote with message and force", []string{"promotion", "environment", "promote", cliEnvironmentID, "--commit-message", "sync", "--force"}, "/promote", `{"commitMessage":"sync","force":true}`, cliEnvPromoteBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := promotionFixture(t, tc.response)
			got := f.run(append(tc.args, "--yes", "--output", "json")...)
			if got.code != ExitSuccess || !sameJSON(t, got.stdout, tc.response) {
				t.Fatalf("json result = %+v", got)
			}
			req := f.lastRequest()
			if req.Method != http.MethodPost || req.URL.EscapedPath() != cliEnvironmentPath+tc.path || f.lastBody() != tc.body {
				t.Errorf("request = %s %s body=%s", req.Method, req.URL.EscapedPath(), f.lastBody())
			}
			got = f.run(append(tc.args, "--yes")...)
			if got.code != ExitSuccess || !strings.Contains(got.stdout, cliEnvironmentID) || !strings.Contains(got.stdout, "Environment:") {
				t.Errorf("text = %+v", got)
			}
		})
	}
}

func TestPromotionEnvironmentApplyTextShowsNewCounts(t *testing.T) {
	f := promotionFixture(t, cliEnvAppliedBody)
	got := f.run("promotion", "environment", "apply", cliEnvironmentID, "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Applied:", "Environment:", "folders      updated      3", "folders      moved        2", "folders      renamed      1", "workflows    moved        2", "credentials  updated      5"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, "Config:") {
		t.Errorf("stdout = %s, want no Config line for an environment result", got.stdout)
	}

	// n8n 2.45 project apply: Config line, and no 2.46 count rows.
	f.body = cliPromotionAppliedBody
	got = f.run("promotion", "project", "apply", "p", "--workflow-id", "w", "--yes")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "Config:") || !strings.Contains(got.stdout, "cfg-apply") {
		t.Fatalf("old shape = %+v", got)
	}
	for _, absent := range []string{"Environment:", "folders      updated", "folders      moved", "folders      renamed", "workflows    moved", "credentials  updated"} {
		if strings.Contains(got.stdout, absent) {
			t.Errorf("old shape stdout invents %q:\n%s", absent, got.stdout)
		}
	}

	// n8n 2.46 project apply renders the new shape the same way.
	f.body = cliEnvAppliedBody
	got = f.run("promotion", "project", "apply", "p", "--workflow-id", "w", "--yes")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "Environment:") || !strings.Contains(got.stdout, "folders      moved        2") {
		t.Errorf("new project shape = %+v", got)
	}
}

func TestPromotionEnvironmentApplyStatuses(t *testing.T) {
	for _, args := range [][]string{
		{"promotion", "environment", "apply", cliEnvironmentID},
		append([]string{"promotion", "environment", "apply-continue", cliEnvironmentID}, envExpectedFlags()...),
	} {
		for _, tc := range []struct {
			body  string
			code  int
			hints []string
		}{
			{cliEnvAppliedBody, ExitSuccess, nil},
			{cliEnvBlockedBody, ExitPromotionBlocked, []string{"n8n promotion environment apply-continue " + cliEnvironmentID, "environmentId=" + cliEnvironmentID, "environment " + cliEnvironmentID + " was blocked"}},
			{cliEnvSourceChangedBody, ExitPromotionSourceChanged, []string{"Review the branch again", "n8n promotion environment apply " + cliEnvironmentID}},
			{`{"status":"queued"}`, ExitError, []string{"does not know"}},
		} {
			f := promotionFixture(t, tc.body)
			got := f.run(append(args, "--yes", "--output", "json")...)
			if got.code != tc.code || !sameJSON(t, got.stdout, tc.body) {
				t.Errorf("%s: result = %+v, want exit %d", args[2], got, tc.code)
			}
			for _, hint := range tc.hints {
				if !strings.Contains(got.stderr, hint) {
					t.Errorf("%s: stderr missing %q:\n%s", args[2], hint, got.stderr)
				}
			}
			if strings.Contains(got.stderr, "configId=") {
				t.Errorf("%s: stderr = %s, want the environmentId hint only", args[2], got.stderr)
			}
			if got = f.run(append(args, "--yes")...); got.code != tc.code {
				t.Errorf("%s text: exit = %d, want %d", args[2], got.code, tc.code)
			}
		}
	}
}

func TestPromotionEnvironmentSyncConfirmationBeforeTransport(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"promotion", "environment", "apply", cliEnvironmentID}, "this cannot be undone"},
		{append([]string{"promotion", "environment", "apply-continue", cliEnvironmentID, "--confirm-destructive-changes"}, envExpectedFlags()...), "ALL team-project deletions"},
		{[]string{"promotion", "environment", "promote", cliEnvironmentID}, "server-generated commit message"},
		{[]string{"promotion", "environment", "promote", cliEnvironmentID, "--commit-message", "sync", "--force"}, "Remote history is rewritten"},
	} {
		t.Run(strings.Join(tc.args[:3], " "), func(t *testing.T) {
			f := promotionFixture(t, cliEnvAppliedBody)
			before := f.requestCount()
			f.stdin = "no\n"
			got := f.run(tc.args...)
			if got.code == ExitSuccess || f.requestCount() != before || !strings.Contains(got.stderr, tc.want) || !strings.Contains(got.stderr, "aborted") {
				t.Errorf("declined = %+v, requests = %d want %d", got, f.requestCount(), before)
			}
			f.interactive = false
			got = f.run(tc.args...)
			if got.code == ExitSuccess || f.requestCount() != before || !strings.Contains(got.stderr, "--yes") {
				t.Errorf("non-interactive = %+v, requests = %d want %d", got, f.requestCount(), before)
			}
		})
	}
}

func TestPromotionEnvironmentSyncRejectsBeforeTransport(t *testing.T) {
	f := promotionFixture(t, cliEnvAppliedBody)
	configFile := writePromotionInput(t, f, "old-changes.json", cliSelectionChanges)
	bothFile := writePromotionInput(t, f, "both.json", strings.Replace(cliEnvChanges, `"branchName":"main"`, `"configId":"cfg","branchName":"main"`, 1))
	before := f.requestCount()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"promotion", "environment", "apply", "conn-1", "--yes"}, "must be a UUID"},
		{[]string{"promotion", "environment", "promote", "conn-1", "--yes"}, "must be a UUID"},
		{[]string{"promotion", "environment", "apply-continue", cliEnvironmentID, "--yes"}, "--expected-environment-id"},
		{[]string{"promotion", "environment", "apply", cliEnvironmentID, "--expected-config-id", "cfg", "--yes"}, "unknown flag"},
		{[]string{"promotion", "environment", "apply", cliEnvironmentID, "--expected-environment-id", cliEnvironmentID, "--yes"}, "branch name is required"},
		{[]string{"promotion", "environment", "apply-continue", cliEnvironmentID, "--expected-from", configFile, "--yes"}, "only n8n 2.45 and earlier return"},
		{[]string{"promotion", "environment", "apply", cliEnvironmentID, "--expected-from", bothFile, "--yes"}, "both present"},
		{[]string{"promotion", "environment", "apply", cliEnvironmentID, "--input", "-"}, "--yes"},
		{[]string{"promotion", "environment", "apply-continue", cliEnvironmentID, "--expected-from", "-"}, "--yes"},
		{[]string{"promotion", "environment", "promote", cliEnvironmentID, "--commit-message", "", "--yes"}, "commit message is required"},
		{[]string{"promotion", "environment", "promote", cliEnvironmentID, "--commit-message", strings.Repeat("m", 1001), "--yes"}, "at most 1000"},
		{[]string{"promotion", "environment", "promote", cliEnvironmentID, "--output", "yaml", "--yes"}, "output"},
	} {
		got := f.run(tc.args...)
		if got.code == ExitSuccess || !strings.Contains(got.stderr, tc.want) || f.requestCount() != before {
			t.Errorf("%v: result = %+v, want %q and no request", tc.args, got, tc.want)
		}
	}
}

func TestPromotionEnvironmentApplyInputAndExpectedFrom(t *testing.T) {
	f := promotionFixture(t, cliEnvAppliedBody)
	f.stdin = `{"expectedSource":` + cliEnvSource + `,"confirmDestructiveChanges":false}`
	got := f.run("promotion", "environment", "apply-continue", cliEnvironmentID, "--input", "-", "--yes")
	if got.code != ExitSuccess || f.lastBody() != f.stdin {
		t.Fatalf("stdin = %+v body=%s", got, f.lastBody())
	}
	f.stdin = `{}`
	got = f.run("promotion", "environment", "apply", cliEnvironmentID, "--input", "-", "--yes")
	if got.code != ExitSuccess || f.lastBody() != `{}` {
		t.Fatalf("empty input = %+v body=%s", got, f.lastBody())
	}
	for _, review := range []string{cliEnvChanges, cliEnvBlockedBody} {
		path := writePromotionInput(t, f, "review.json", review)
		got = f.run("promotion", "environment", "apply-continue", cliEnvironmentID, "--expected-from", path, "--yes")
		if got.code != ExitSuccess || f.lastBody() != `{"expectedSource":`+cliEnvSource+`}` {
			t.Errorf("expected-from = %+v body=%s", got, f.lastBody())
		}
	}
}

// Project routes serve both generations: pass through whichever identifier the
// instance returned, and keep the two flags apart.
func TestPromotionProjectApplyBothExpectedSources(t *testing.T) {
	f := promotionFixture(t, cliEnvAppliedBody)
	got := f.run(append([]string{"promotion", "project", "apply", "p", "--workflow-id", "w", "--yes"}, envExpectedFlags()...)...)
	if got.code != ExitSuccess || f.lastBody() != `{"workflowIds":["w"],"expectedSource":`+cliEnvSource+`}` {
		t.Errorf("environment flag = %+v body=%s", got, f.lastBody())
	}
	got = f.run(append([]string{"promotion", "project", "apply-continue", "p", "--workflow-id", "w", "--yes"}, selectionExpectedFlags()...)...)
	if got.code != ExitSuccess || f.lastBody() != `{"workflowIds":["w"],"expectedSource":`+cliSelectionSource+`}` {
		t.Errorf("config flag = %+v body=%s", got, f.lastBody())
	}
	for _, review := range []string{cliEnvChanges, cliEnvBlockedBody} {
		path := writePromotionInput(t, f, "review.json", review)
		got = f.run("promotion", "project", "apply-continue", "p", "--workflow-id", "w", "--expected-from", path, "--yes")
		if got.code != ExitSuccess || f.lastBody() != `{"workflowIds":["w"],"expectedSource":`+cliEnvSource+`}` {
			t.Errorf("2.46 expected-from = %+v body=%s", got, f.lastBody())
		}
	}
	path := writePromotionInput(t, f, "old-review.json", cliSelectionChanges)
	got = f.run("promotion", "project", "apply", "p", "--workflow-id", "w", "--expected-from", path, "--yes")
	if got.code != ExitSuccess || f.lastBody() != `{"workflowIds":["w"],"expectedSource":`+cliSelectionSource+`}` {
		t.Errorf("2.45 expected-from = %+v body=%s", got, f.lastBody())
	}

	before := f.requestCount()
	got = f.run(append([]string{"promotion", "project", "apply", "p", "--workflow-id", "w", "--expected-config-id", "cfg", "--yes"}, envExpectedFlags()...)...)
	if got.code == ExitSuccess || f.requestCount() != before ||
		!strings.Contains(got.stderr, "mutually exclusive") || !strings.Contains(got.stderr, "n8n 2.46 and later") || !strings.Contains(got.stderr, "n8n 2.45 and earlier") {
		t.Errorf("both flags = %+v, want the per-version explanation and no request", got)
	}
	got = f.run("promotion", "project", "apply-continue", "p", "--workflow-id", "w", "--expected-branch", "main", "--expected-commit", cliSelectionSHA, "--yes")
	if got.code == ExitSuccess || f.requestCount() != before || !strings.Contains(got.stderr, "--expected-environment-id") || !strings.Contains(got.stderr, "--expected-config-id") {
		t.Errorf("no id = %+v", got)
	}
	// The connection apply-continue predates environments.
	got = f.run(append([]string{"promotion", "apply-continue", "c", "--yes"}, envExpectedFlags()...)...)
	if got.code == ExitSuccess || f.requestCount() != before || !strings.Contains(got.stderr, "unknown flag") {
		t.Errorf("connection environment flag = %+v", got)
	}
	envFile := writePromotionInput(t, f, "env-review.json", cliEnvChanges)
	got = f.run("promotion", "apply-continue", "c", "--expected-from", envFile, "--yes")
	if got.code == ExitSuccess || f.requestCount() != before || !strings.Contains(got.stderr, "n8n promotion environment apply-continue") {
		t.Errorf("connection environment source = %+v", got)
	}
}

func TestPromotionPromoteReviewAndWarnings(t *testing.T) {
	for _, args := range [][]string{
		{"promotion", "environment", "promote", cliEnvironmentID},
		{"promotion", "project", "promote", "p", "--workflow-id", "w"},
	} {
		f := promotionFixture(t, cliEnvPromoteBody)
		got := f.run(append(args, "--yes")...)
		if got.code != ExitSuccess {
			t.Fatalf("%v: exit = %d, stderr: %s", args, got.code, got.stderr)
		}
		for _, want := range []string{"Promoted:", cliEnvironmentID, "n8n/promotion-1", "Review:", "#42 https://gitlab.example.com/org/repo/-/merge_requests/42", "workflows"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("%v: stdout missing %q:\n%s", args, want, got.stdout)
			}
		}
		for _, want := range []string{"Warning: Branch protection blocks merging (see https://gitlab.example.com/help)", "Warning: No reviewers\n"} {
			if !strings.Contains(got.stderr, want) {
				t.Errorf("%v: stderr missing %q:\n%s", args, want, got.stderr)
			}
		}
		if strings.Contains(got.stdout, "Warning") {
			t.Errorf("%v: stdout = %s, want warnings on stderr only", args, got.stdout)
		}
		got = f.run(append(args, "--yes", "--output", "json")...)
		if got.code != ExitSuccess || !sameJSON(t, got.stdout, cliEnvPromoteBody) || !strings.Contains(got.stderr, "Warning: No reviewers") {
			t.Errorf("%v json = %+v", args, got)
		}
	}
	// n8n 2.45: no review line, no warnings, identifiers kept as sent.
	f := promotionFixture(t, cliPromotionPromoteBody)
	got := f.run("promotion", "project", "promote", "p", "--workflow-id", "w", "--yes")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "conn-1") || strings.Contains(got.stdout, "Review:") || strings.Contains(got.stdout, "Environment:") || got.stderr != "" {
		t.Errorf("old shape = %+v", got)
	}
	got = f.run("promotion", "project", "promote", "p", "--workflow-id", "w", "--yes", "--output", "json")
	if got.code != ExitSuccess || !sameJSON(t, got.stdout, cliPromotionPromoteBody) {
		t.Errorf("old shape json = %+v", got)
	}
}

func TestPromotionChangesEnvironmentSourceText(t *testing.T) {
	f := promotionFixture(t, cliEnvChanges)
	got := f.run("promotion", "project", "changes", "p", "apply")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "Environment:") || !strings.Contains(got.stdout, cliEnvironmentID) || strings.Contains(got.stdout, "Config:") {
		t.Errorf("2.46 changes = %+v", got)
	}
	got = f.run("promotion", "project", "changes", "p", "apply", "--output", "json")
	if got.code != ExitSuccess || !sameJSON(t, got.stdout, cliEnvChanges) {
		t.Errorf("2.46 changes json = %+v", got)
	}
	f.body = cliSelectionChanges
	got = f.run("promotion", "project", "changes", "p", "apply")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "Config:") || strings.Contains(got.stdout, "Environment:") {
		t.Errorf("2.45 changes = %+v", got)
	}
}

func TestPromotionEnvironmentSyncErrorsNameTheFix(t *testing.T) {
	f := promotionFixture(t, `{}`)
	// A bare 404: the instance may predate environments.
	f.route = func(*http.Request, int) (int, string) { return http.StatusNotFound, `{"message":"not found"}` }
	for _, args := range [][]string{
		{"promotion", "environment", "apply", cliEnvironmentID, "--yes"},
		append([]string{"promotion", "environment", "apply-continue", cliEnvironmentID, "--yes"}, envExpectedFlags()...),
		{"promotion", "environment", "promote", cliEnvironmentID, "--yes"},
	} {
		got := f.run(args...)
		if got.code != ExitError || !strings.Contains(got.stderr, "older than n8n 2.46") || !strings.Contains(got.stderr, "'n8n promotion connection'") || !strings.Contains(got.stderr, "n8n promotion environment list") {
			t.Errorf("%s 404 = %+v, want the connection and list hints", args[2], got)
		}
	}
	// And the connection commands point at environments.
	got := f.run("promotion", "apply", "conn-1", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "'n8n promotion environment'") {
		t.Errorf("connection apply 404 = %+v", got)
	}
	f.route = nil

	f.status = http.StatusForbidden
	for scope, args := range map[string][]string{
		"gitConnection:pull": {"promotion", "environment", "apply", cliEnvironmentID, "--yes"},
		"gitConnection:push": {"promotion", "environment", "promote", cliEnvironmentID, "--yes"},
	} {
		if got := f.run(args...); got.code != ExitError || !strings.Contains(got.stderr, scope) {
			t.Errorf("%v 403 = %+v, want %s", args, got, scope)
		}
	}
	for action, want := range map[string]string{"environment apply": "gitConnection:pull", "environment apply-continue": "gitConnection:pull", "environment promote": "gitConnection:push"} {
		if got := promotionScope(action); got != want {
			t.Errorf("scope(%s) = %s, want %s", action, got, want)
		}
	}

	f.status = http.StatusConflict
	f.body = `{"message":"moved"}`
	for _, args := range [][]string{
		{"promotion", "environment", "apply", cliEnvironmentID, "--yes"},
		{"promotion", "project", "apply", "p", "--workflow-id", "w", "--yes"},
	} {
		if got := f.run(args...); got.code != ExitError || !strings.Contains(got.stderr, "different projects") || !strings.Contains(got.stderr, "nothing was applied") {
			t.Errorf("%v 409 = %+v", args, got)
		}
	}
	if got := f.run("promotion", "environment", "promote", cliEnvironmentID, "--yes"); got.code != ExitError || !strings.Contains(got.stderr, "--force") || !strings.Contains(got.stderr, "nothing was pushed") {
		t.Errorf("promote 409 = %+v", got)
	}

	f.status = http.StatusBadRequest
	for _, args := range [][]string{
		{"promotion", "environment", "promote", cliEnvironmentID, "--yes"},
		{"promotion", "project", "promote", "p", "--workflow-id", "w", "--yes"},
	} {
		if got := f.run(args...); got.code != ExitError || !strings.Contains(got.stderr, "personal project") || !strings.Contains(got.stderr, "move it back") {
			t.Errorf("%v 400 = %+v", args, got)
		}
	}

	f.status = http.StatusUnprocessableEntity
	if got := f.run("promotion", "environment", "apply", cliEnvironmentID, "--yes"); got.code != ExitError || !strings.Contains(got.stderr, "cannot be imported") {
		t.Errorf("apply 422 = %+v, want the import diagnostic", got)
	}
}

func TestPromotionHelpNamesEnvironmentSync(t *testing.T) {
	f := promotionFixture(t, `{}`)
	got := f.run("promotion", "--help")
	if got.code != ExitSuccess || strings.Contains(got.stdout, "still address connections") ||
		!strings.Contains(got.stdout, "environment apply") || !strings.Contains(got.stdout, "environment promote") {
		t.Errorf("group help = %s", got.stdout)
	}
	got = f.run("promotion", "environment", "--help")
	for _, want := range []string{"apply", "apply-continue", "promote"} {
		if !strings.Contains(got.stdout, "  "+want+" ") {
			t.Errorf("environment help lacks %q:\n%s", want, got.stdout)
		}
	}
}
