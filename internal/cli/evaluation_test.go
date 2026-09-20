package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliEvaluationRun = `{
  "id":"run-1","status":"completed","runAt":"2026-09-18T10:00:00Z",
  "completedAt":"2026-09-18T10:01:00Z","metrics":{"accuracy":0.95},
  "errorCode":null,"errorDetails":null,"finalResult":"success","testCaseCount":2,
  "createdAt":"2026-09-18T09:59:00Z","updatedAt":"2026-09-18T10:01:00Z"
}`

const cliEvaluationCase = `{
  "id":"case-1","status":"success","runAt":"2026-09-18T10:00:00Z",
  "completedAt":"2026-09-18T10:00:10Z","metrics":{"quality":1},
  "errorCode":null,"errorDetails":null,"inputs":{"question":"hello"},
  "outputs":{"answer":"world"},"executionId":"1000"
}`

func evaluationFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestEvaluationListTextJSONAndQuery(t *testing.T) {
	f := evaluationFixture(t, `{"data":[`+cliEvaluationRun+`],"nextCursor":"next/one?x=1"}`)
	got := f.run("evaluation", "list", "wf/one?x=1", "--limit", "50", "--cursor", "prev/one?x=1", "--status", "completed")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Workflow:", "wf/one?x=1", "Evaluation runs:", "run-1", "completed", "success", "Next cursor:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+"/workflows/wf%2Fone%3Fx=1/test-runs" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	if req.URL.Query().Get("limit") != "50" || req.URL.Query().Get("cursor") != "prev/one?x=1" || req.URL.Query().Get("status") != "completed" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}

	got = f.run("evaluation", "list", "wf-1", "--output", "json")
	var page n8n.Page[n8n.EvaluationRunSummary]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 || page.Data[0].ID != "run-1" {
		t.Errorf("stdout = %q, want run page (error: %v)", got.stdout, err)
	}
}

func TestEvaluationListAllKeepsStatus(t *testing.T) {
	f := evaluationFixture(t, "")
	f.bodyFunc = func(page int) string {
		if page == 0 {
			return `{"data":[` + cliEvaluationRun + `],"nextCursor":"next"}`
		}
		return `{"data":[{"id":"run-2","status":"completed","testCaseCount":0}],"nextCursor":null}`
	}
	got := f.run("evaluation", "list", "wf-1", "--all", "--limit", "1", "--status", "completed", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.EvaluationRunSummary]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 2 || page.NextCursor != "" {
		t.Errorf("stdout = %q, want two collected runs (error: %v)", got.stdout, err)
	}
	second := f.lastRequest()
	if second.URL.Query().Get("cursor") != "next" || second.URL.Query().Get("status") != "completed" || second.URL.Query().Get("limit") != "1" {
		t.Errorf("second query = %q", second.URL.RawQuery)
	}
}

func TestEvaluationCreateAndGet(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		f := evaluationFixture(t, `{"id":"run-1","status":"new","createdAt":"2026-09-18T09:59:00Z"}`)
		got := f.run("evaluation", "create", "wf/one?x=1")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+"/workflows/wf%2Fone%3Fx=1/test-runs" || f.lastBody() != "" {
			t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), f.lastBody())
		}
		for _, want := range []string{"Evaluation run:", "run-1", "Status:", "new", "Next:"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("stdout missing %q: %s", want, got.stdout)
			}
		}
	})

	t.Run("get JSON", func(t *testing.T) {
		f := evaluationFixture(t, cliEvaluationRun)
		got := f.run("evaluation", "get", "wf/one?x=1", "run/two?x=2", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+"/workflows/wf%2Fone%3Fx=1/test-runs/run%2Ftwo%3Fx=2" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		var run n8n.EvaluationRunSummary
		if err := json.Unmarshal([]byte(got.stdout), &run); err != nil || run.ID != "run-1" || !strings.Contains(string(run.Metrics), `"accuracy"`) {
			t.Errorf("stdout = %q (error: %v)", got.stdout, err)
		}
	})
}

func TestEvaluationCancelConfirmationConflictAndResult(t *testing.T) {
	f := evaluationFixture(t, `{"id":"run-1","status":"cancelled"}`)
	before := f.requestCount()
	f.stdin = "no\n"
	got := f.run("evaluation", "cancel", "wf-1", "run-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "cannot resume") || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v", got)
	}

	f.interactive = false
	got = f.run("evaluation", "cancel", "wf-1", "run-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v", got)
	}

	got = f.run("evaluation", "cancel", "wf/one?x=1", "run/two?x=2", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+"/workflows/wf%2Fone%3Fx=1/test-runs/run%2Ftwo%3Fx=2/cancel" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	var result n8n.EvaluationCancelResult
	if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Status != n8n.EvaluationRunCancelled {
		t.Errorf("stdout = %q (error: %v)", got.stdout, err)
	}

	f.status = http.StatusConflict
	f.body = `{"message":"already finished"}`
	got = f.run("evaluation", "cancel", "wf-1", "run-1", "--yes")
	if got.code != ExitError || !strings.Contains(got.stderr, "not cancellable") || !strings.Contains(got.stderr, "evaluation get wf-1 run-1") {
		t.Errorf("conflict result = %+v", got)
	}
}

func TestEvaluationCaseListPaginationAndJSON(t *testing.T) {
	f := evaluationFixture(t, "")
	f.bodyFunc = func(page int) string {
		if page == 0 {
			return `{"data":[` + cliEvaluationCase + `],"nextCursor":"next"}`
		}
		return `{"data":[{"id":"case-2","status":"warning","runAt":null,"completedAt":null,"executionId":null}],"nextCursor":null}`
	}
	got := f.run("evaluation", "case", "list", "wf/one?x=1", "run/two?x=2", "--all", "--limit", "1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.EvaluationTestCase]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 2 || page.Data[0].ID != "case-1" || !strings.Contains(string(page.Data[0].Inputs), `"question"`) {
		t.Errorf("stdout = %q (error: %v)", got.stdout, err)
	}
	first := f.requests[len(f.requests)-2]
	if first.URL.EscapedPath() != n8n.BasePath+"/workflows/wf%2Fone%3Fx=1/test-runs/run%2Ftwo%3Fx=2/test-cases" || first.URL.Query().Get("limit") != "1" {
		t.Errorf("first request = %s?%s", first.URL.EscapedPath(), first.URL.RawQuery)
	}
	second := f.lastRequest()
	if second.URL.Query().Get("cursor") != "next" || second.URL.Query().Get("limit") != "1" {
		t.Errorf("second query = %q", second.URL.RawQuery)
	}
}

func TestEvaluationValidationBeforeTransport(t *testing.T) {
	for name, args := range map[string][]string{
		"status":        {"evaluation", "list", "wf-1", "--status", "success"},
		"run limit":     {"evaluation", "list", "wf-1", "--limit", "251"},
		"case limit":    {"evaluation", "case", "list", "wf-1", "run-1", "--limit", "251"},
		"workflow ID":   {"evaluation", "create", " "},
		"run ID":        {"evaluation", "get", "wf-1", " "},
		"output format": {"evaluation", "get", "wf-1", "run-1", "--output", "xml"},
	} {
		t.Run(name, func(t *testing.T) {
			f := evaluationFixture(t, `{"data":[],"nextCursor":null}`)
			before := f.requestCount()
			got := f.run(args...)
			if got.code != ExitError || got.stderr == "" {
				t.Errorf("result = %+v", got)
			}
			if f.requestCount() != before {
				t.Error("invalid command reached instance")
			}
		})
	}
}

func TestEvaluationAPIErrorsExplainFix(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		want   []string
	}{
		{name: "license", status: http.StatusPaymentRequired, args: []string{"evaluation", "create", "wf-1"}, want: []string{"does not license", "discover --resource evaluation"}},
		{name: "execute permission", status: http.StatusForbidden, args: []string{"evaluation", "create", "wf-1"}, want: []string{"workflow:execute", "testRun:create"}},
		{name: "missing workflow", status: http.StatusNotFound, args: []string{"evaluation", "list", "missing"}, want: []string{"no accessible workflow", "workflow list"}},
		{name: "missing run", status: http.StatusNotFound, args: []string{"evaluation", "get", "wf-1", "missing"}, want: []string{"no evaluation run", "evaluation list wf-1"}},
		{name: "create conflict", status: http.StatusConflict, args: []string{"evaluation", "create", "wf-1"}, want: []string{"current run state conflicts", "evaluation list wf-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := evaluationFixture(t, `{"message":"denied"}`)
			f.status = tt.status
			got := f.run(tt.args...)
			if got.code != ExitError {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr missing %q: %s", want, got.stderr)
				}
			}
		})
	}
}
