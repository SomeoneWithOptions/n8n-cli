package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliSelectionSHA = "0123456789012345678901234567890123456789"
const cliSelectionSource = `{"configId":"cfg","branchName":"main","commitSha":"` + cliSelectionSHA + `"}`
const cliSelectionChanges = `{"commitSha":"` + cliSelectionSHA + `","source":{"configId":"cfg","branchName":"main"},"changes":[{"id":"w","name":"Flow","type":"workflow","status":"deleted","version":null,"updatedAt":null,"updatedBy":null,"dependencyCount":1}],"future":true}`

func selectionExpectedFlags() []string {
	return []string{"--expected-config-id", "cfg", "--expected-branch", "main", "--expected-commit", cliSelectionSHA}
}

func TestPromotionSelectionCommands(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		args                 []string
		body, path, response string
	}{
		{"apply", []string{"promotion", "project", "apply", "p/1", "--workflow-id", "w", "--workflow-id", "w2"}, `{"workflowIds":["w","w2"]}`, "/promotions/projects/p%2F1/apply", cliPromotionAppliedBody},
		{"promote", []string{"promotion", "project", "promote", "p/1", "--workflow-id", "w"}, `{"workflowIds":["w"]}`, "/promotions/projects/p%2F1/promote", cliPromotionPromoteBody},
		{"continue project", append([]string{"promotion", "project", "apply-continue", "p/1", "--workflow-id", "w", "--confirm-destructive-changes=false"}, selectionExpectedFlags()...), `{"workflowIds":["w"],"expectedSource":` + cliSelectionSource + `,"confirmDestructiveChanges":false}`, "/promotions/projects/p%2F1/apply/continue", cliPromotionAppliedBody},
		{"continue connection", append([]string{"promotion", "apply-continue", "c/1", "--confirm-destructive-changes"}, selectionExpectedFlags()...), `{"expectedSource":` + cliSelectionSource + `,"confirmDestructiveChanges":true}`, "/promotions/connections/c%2F1/apply/continue", cliPromotionAppliedBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := promotionFixture(t, tc.response)
			got := f.run(append(tc.args, "--yes", "--output", "json")...)
			if got.code != ExitSuccess || !sameJSON(t, got.stdout, tc.response) {
				t.Fatalf("result=%+v", got)
			}
			req := f.lastRequest()
			if req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+tc.path || f.lastBody() != tc.body {
				t.Errorf("request=%s %s body=%s", req.Method, req.URL.EscapedPath(), f.lastBody())
			}
			got = f.run(append(tc.args, "--yes")...)
			if got.code != ExitSuccess || !strings.Contains(got.stdout, "workflows") {
				t.Errorf("text=%+v", got)
			}
		})
	}
}

func TestPromotionSelectionConfirmationBeforeTransport(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"promotion", "project", "apply", "p", "--workflow-id", "w"}, "absent from the branch are deleted"},
		{[]string{"promotion", "project", "promote", "p", "--workflow-id", "w"}, "remote branch is committed and pushed"},
		{append([]string{"promotion", "apply-continue", "c", "--confirm-destructive-changes"}, selectionExpectedFlags()...), "ALL team-project deletions with everything in them"},
		{append([]string{"promotion", "project", "apply-continue", "p", "--workflow-id", "w", "--confirm-destructive-changes"}, selectionExpectedFlags()...), "ALL destructive data-table column changes"},
	} {
		t.Run(strings.Join(tc.args[:3], " "), func(t *testing.T) {
			f := promotionFixture(t, cliPromotionAppliedBody)
			before := f.requestCount()
			f.stdin = "no\n"
			got := f.run(tc.args...)
			if got.code == ExitSuccess || f.requestCount() != before || !strings.Contains(got.stderr, tc.want) {
				t.Errorf("refusal=%+v requests=%d want=%d", got, f.requestCount(), before)
			}
			f.interactive = false
			got = f.run(tc.args...)
			if got.code == ExitSuccess || f.requestCount() != before || !strings.Contains(got.stderr, "--yes") {
				t.Errorf("noninteractive=%+v", got)
			}
		})
	}
}

func TestPromotionSelectionInputAndStrictValidation(t *testing.T) {
	for _, tc := range []struct {
		args []string
		body string
	}{
		{[]string{"promotion", "project", "apply", "p"}, `{"workflowIds":["w"],"expectedSource":` + cliSelectionSource + `}`},
		{[]string{"promotion", "project", "promote", "p"}, `{"workflowIds":["w"],"commitMessage":"sync"}`},
		{[]string{"promotion", "project", "apply-continue", "p"}, `{"workflowIds":["w"],"expectedSource":` + cliSelectionSource + `,"confirmDestructiveChanges":true}`},
		{[]string{"promotion", "apply-continue", "c"}, `{"expectedSource":` + cliSelectionSource + `,"confirmDestructiveChanges":true}`},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			f := promotionFixture(t, cliPromotionAppliedBody)
			if tc.args[2] == "promote" {
				f.body = cliPromotionPromoteBody
			}
			f.stdin = tc.body
			before := f.requestCount()
			got := f.run(append(tc.args, "--input", "-")...)
			if got.code == ExitSuccess || f.requestCount() != before || !strings.Contains(got.stderr, "--yes") {
				t.Fatalf("stdin refusal=%+v", got)
			}
			got = f.run(append(tc.args, "--input", "-", "--yes")...)
			if got.code != ExitSuccess || f.lastBody() != tc.body {
				t.Fatalf("stdin=%+v body=%s", got, f.lastBody())
			}
			path := writePromotionInput(t, f, "request.json", tc.body)
			got = f.run(append(tc.args, "--input", path, "--yes")...)
			if got.code != ExitSuccess {
				t.Errorf("file=%+v", got)
			}
			before = f.requestCount()
			for _, body := range []string{`{}`, `null`, tc.body + ` {}`, strings.TrimSuffix(tc.body, "}") + `,"unknown":1}`, strings.Repeat(" ", maxPromotionDocument+1), `{"workflowIds":["w","w"]}`} {
				f.stdin = body
				got = f.run(append(tc.args, "--input", "-", "--yes")...)
				if got.code == ExitSuccess || f.requestCount() != before {
					t.Errorf("invalid accepted: %+v", got)
				}
			}
			flag := "--expected-config-id"
			if strings.Contains(strings.Join(tc.args, " "), "promote") {
				flag = "--commit-message"
			}
			got = f.run(append(tc.args, "--input", path, flag, "", "--yes")...)
			if got.code == ExitSuccess || !strings.Contains(got.stderr, "mutually exclusive") || f.requestCount() != before {
				t.Errorf("mixed input=%+v", got)
			}
		})
	}
}

func TestPromotionExpectedFrom(t *testing.T) {
	for _, body := range []string{cliSelectionChanges, cliPromotionBlockedBody} {
		f := promotionFixture(t, cliPromotionAppliedBody)
		path := writePromotionInput(t, f, "review.json", body)
		got := f.run("promotion", "project", "apply-continue", "p", "--workflow-id", "w", "--expected-from", path, "--yes")
		if got.code != ExitSuccess {
			t.Fatal(got.stderr)
		}
		var request n8n.ContinueApplyProjectSelectionRequest
		if err := json.Unmarshal([]byte(f.lastBody()), &request); err != nil {
			t.Fatal(err)
		}
		if request.ExpectedSource.BranchName != "main" || request.ExpectedSource.Validate() != nil || len(request.WorkflowIDs) != 1 {
			t.Errorf("request=%+v", request)
		}
		f.stdin = body
		before := f.requestCount()
		got = f.run("promotion", "apply-continue", "c", "--expected-from", "-")
		if got.code == ExitSuccess || f.requestCount() != before || !strings.Contains(got.stderr, "--yes") {
			t.Errorf("stdin refusal=%+v", got)
		}
		got = f.run("promotion", "apply-continue", "c", "--expected-from", "-", "--yes")
		if got.code != ExitSuccess {
			t.Errorf("stdin=%+v", got)
		}
	}
	f := promotionFixture(t, cliPromotionAppliedBody)
	before := f.requestCount()
	for _, body := range []string{
		`null`, `{}`, cliSelectionChanges + ` {}`,
		strings.Replace(cliSelectionChanges, `"commitSha":"`+cliSelectionSHA+`"`, `"commitSha":null`, 1),
		strings.Replace(cliSelectionChanges, cliSelectionSHA, "short", 1),
		strings.Replace(cliSelectionChanges, `"configId":"cfg"`, `"configId":17`, 1),
		strings.Replace(cliSelectionChanges, `"branchName":"main"`, `"branchName":null`, 1),
		cliPromotionAppliedBody, cliPromotionSourceChangedBody,
		strings.Repeat(" ", maxPromotionDocument+1),
	} {
		f.stdin = body
		got := f.run("promotion", "apply-continue", "c", "--expected-from", "-", "--yes")
		if got.code == ExitSuccess || f.requestCount() != before {
			t.Errorf("bad review accepted: %+v", got)
		}
	}
	f.stdin = cliSelectionChanges
	got := f.run("promotion", "apply-continue", "c", "--expected-from", "-", "--expected-config-id", "cfg", "--yes")
	if got.code == ExitSuccess || !strings.Contains(got.stderr, "mutually exclusive") || f.requestCount() != before {
		t.Errorf("mixed source=%+v", got)
	}
}

func TestPromotionSelectionStatuses(t *testing.T) {
	for _, args := range [][]string{
		{"promotion", "project", "apply", "p", "--workflow-id", "w"},
		append([]string{"promotion", "project", "apply-continue", "p", "--workflow-id", "w"}, selectionExpectedFlags()...),
		append([]string{"promotion", "apply-continue", "c"}, selectionExpectedFlags()...),
	} {
		for _, tc := range []struct {
			body string
			code int
			hint string
		}{
			{cliPromotionAppliedBody, ExitSuccess, ""},
			{cliPromotionBlockedBody, ExitPromotionBlocked, "apply-continue"},
			{cliPromotionSourceChangedBody, ExitPromotionSourceChanged, "Review the branch again"},
			{`{"status":"queued","future":true}`, ExitError, "does not know"},
		} {
			t.Run(strings.Join(args[:3], " ")+fmt.Sprint(tc.code), func(t *testing.T) {
				f := promotionFixture(t, tc.body)
				got := f.run(append(args, "--yes", "--output", "json")...)
				if got.code != tc.code || !sameJSON(t, got.stdout, tc.body) || !strings.Contains(got.stderr, tc.hint) {
					t.Errorf("result=%+v", got)
				}
				if tc.code == ExitPromotionBlocked && args[1] == "project" && (!strings.Contains(got.stderr, "promotion project apply-continue p") || !strings.Contains(got.stderr, "same full selection") || !strings.Contains(got.stderr, "project p was blocked")) {
					t.Errorf("project hint=%s", got.stderr)
				}
				got = f.run(append(args, "--yes")...)
				if got.code != tc.code {
					t.Errorf("text=%+v", got)
				}
			})
		}
	}
}

func TestPromotionDiscoveryAndChangesCLI(t *testing.T) {
	for _, branches := range []bool{false, true} {
		t.Run(fmt.Sprint(branches), func(t *testing.T) {
			f := promotionFixture(t, `{}`)
			f.route = func(r *http.Request, page int) (int, string) {
				if r.URL.Query().Get("search") != "a & b" || r.URL.Query().Get("limit") != "100" {
					t.Errorf("query=%s", r.URL.RawQuery)
				}
				cursor := "next"
				if page%2 == 1 {
					cursor = ""
					if r.URL.Query().Get("cursor") != "next" {
						t.Errorf("cursor=%s", r.URL.RawQuery)
					}
				}
				return 200, fmt.Sprintf(`{"data":[{"id":"1","name":"main","fullPath":"org/repo","remoteUrl":"https://gitlab.com/org/repo.git","defaultBranch":null,"isDefault":true}],"nextCursor":%q}`, cursor)
			}
			args := []string{"promotion", "provider", "repositories", "p"}
			if branches {
				args = []string{"promotion", "provider", "branches", "p", "org/repo"}
			}
			args = append(args, "--search", "a & b", "--limit", "100", "--all")
			before := f.requestCount()
			got := f.run(append(args, "--output", "json")...)
			var result struct {
				Data       []json.RawMessage `json:"data"`
				NextCursor string            `json:"nextCursor"`
			}
			if got.code != ExitSuccess || json.Unmarshal([]byte(got.stdout), &result) != nil || len(result.Data) != 2 || result.NextCursor != "" || f.requestCount() != before+2 {
				t.Fatalf("result=%+v", got)
			}
			got = f.run(args...)
			if got.code != ExitSuccess || !strings.Contains(got.stdout, "main") {
				t.Errorf("text=%+v", got)
			}
			if branches && !strings.Contains(f.lastRequest().URL.EscapedPath(), "org%2Frepo") {
				t.Errorf("path=%s", f.lastRequest().URL.EscapedPath())
			}
			f.route = nil
			f.body = `{"data":[],"nextCursor":null}`
			got = f.run(args...)
			if got.code != ExitSuccess || !strings.Contains(got.stderr, "No ") {
				t.Errorf("empty=%+v", got)
			}
		})
	}
	f := promotionFixture(t, cliSelectionChanges)
	got := f.run("promotion", "project", "changes", "p", "apply", "--search", "a & b", "--sort", "status", "--order", "desc", "--output", "json")
	if got.code != ExitSuccess || !sameJSON(t, got.stdout, cliSelectionChanges) {
		t.Fatalf("changes=%+v", got)
	}
	q := f.lastRequest().URL.Query()
	if q.Get("search") != "a & b" || q.Get("sort") != "status" || q.Get("order") != "desc" {
		t.Errorf("query=%v", q)
	}
	got = f.run("promotion", "project", "changes", "p", "promote")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "deleted") || !strings.Contains(got.stdout, cliSelectionSHA) {
		t.Errorf("text=%+v", got)
	}
	f.body = `{"commitSha":null,"source":{"configId":"cfg","branchName":"main"},"changes":[]}`
	got = f.run("promotion", "project", "changes", "p", "apply")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "No workflow changes") {
		t.Errorf("empty=%+v", got)
	}
}

func TestPromotionSelectionInvalidArgumentsAndScopes(t *testing.T) {
	f := promotionFixture(t, `{}`)
	before := f.requestCount()
	for _, args := range [][]string{
		{"promotion", "provider", "repositories"},
		{"promotion", "provider", "branches", "p"},
		{"promotion", "project", "changes", "p", "bad"},
		{"promotion", "project", "changes", "p", "apply", "--sort", "bad"},
		{"promotion", "project", "changes", "p", "apply", "--order", "bad"},
		{"promotion", "project", "apply", "p"},
		{"promotion", "project", "apply", "p", "--workflow-id", "w", "--workflow-id", "w"},
		{"promotion", "project", "apply", "p", "--workflow-id", "w", "--expected-config-id", "cfg"},
		{"promotion", "project", "apply-continue", "p", "--workflow-id", "w"},
		{"promotion", "apply-continue", "c"},
		{"promotion", "project", "promote", "p", "--workflow-id", "w", "--force"},
		{"promotion", "project", "promote", "p", "--workflow-id", "w", "--commit-message", ""},
		{"promotion", "project", "apply", " p", "--workflow-id", "w"},
		{"promotion", "provider", "repositories", "p", "--output", "yaml"},
	} {
		got := f.run(args...)
		if got.code == ExitSuccess || f.requestCount() != before {
			t.Errorf("args=%v result=%+v", args, got)
		}
	}
	for action, want := range map[string]string{"project changes apply": "gitConnection:pull", "project changes promote": "gitConnection:push", "project apply": "gitConnection:pull", "project apply-continue": "gitConnection:pull", "apply-continue": "gitConnection:pull", "project promote": "gitConnection:push", "provider repositories": "gitConnection:discover", "provider branches": "gitConnection:discover"} {
		if got := promotionScope(action); got != want {
			t.Errorf("%s=%s want %s", action, got, want)
		}
	}
	for _, tc := range []struct {
		args  []string
		scope string
	}{
		{[]string{"promotion", "project", "changes", "p", "apply"}, "gitConnection:pull"},
		{[]string{"promotion", "project", "changes", "p", "promote"}, "gitConnection:push"},
		{[]string{"promotion", "provider", "repositories", "p"}, "gitConnection:discover"},
	} {
		f.status = 403
		got := f.run(tc.args...)
		if got.code != ExitError || !strings.Contains(got.stderr, tc.scope) || !strings.Contains(got.stderr, "discover --resource promotions") {
			t.Errorf("403=%+v", got)
		}
	}
}
