package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliExecution = `{
  "id":"1000",
  "finished":true,
  "mode":"manual",
  "retryOf":null,
  "retrySuccessId":null,
  "status":"success",
  "createdAt":"2026-09-17T10:00:00Z",
  "startedAt":"2026-09-17T10:00:01Z",
  "stoppedAt":"2026-09-17T10:00:02Z",
  "deletedAt":null,
  "workflowId":"wf-1",
  "waitTill":null,
  "storedAt":"db",
  "tracingContext":null,
  "deduplicationKey":null,
  "jsonSizeBytes":2048,
  "binaryDataSizeBytes":0,
  "workflowVersionId":"ver-1",
  "usedPrivateCredentials":false,
  "data":{"resultData":{"runData":{}}},
  "workflowData":{"id":"wf-1"},
  "customData":{}
}`

const cliDeletedExecution = `{
  "id":1000,"finished":true,"mode":"manual","retryOf":null,"retrySuccessId":null,
  "status":"success","createdAt":"2026-09-17T10:00:00Z","startedAt":"2026-09-17T10:00:01Z",
  "stoppedAt":"2026-09-17T10:00:02Z","deletedAt":null,"workflowId":"wf-1","waitTill":null,
  "storedAt":"db","tracingContext":null,"deduplicationKey":null,"jsonSizeBytes":2048,
  "binaryDataSizeBytes":0,"workflowVersionId":"ver-1","usedPrivateCredentials":false
}`

func executionFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestExecutionListTextJSONAndQuery(t *testing.T) {
	f := executionFixture(t, `{"data":[`+cliExecution+`],"nextCursor":"next/one?x=1"}`)
	got := f.run("execution", "list", "--limit", "50", "--cursor", "prev/one?x=1",
		"--status", "success", "--workflow-id", "wf-1", "--project-id", "pr-1",
		"--started-after", "2026-09-17T00:00:00Z", "--started-before", "2026-09-18T00:00:00Z",
		"--include-data", "--ignore-data-size-limit", "--redact-execution-data", "false")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Executions:", "1000", "wf-1", "success", "Next cursor:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.ExecutionsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	for field, want := range map[string]string{
		"limit": "50", "cursor": "prev/one?x=1", "status": "success", "workflowId": "wf-1",
		"projectId": "pr-1", "startedAfter": "2026-09-17T00:00:00Z", "startedBefore": "2026-09-18T00:00:00Z",
		"includeData": "true", "ignoreDataSizeLimit": "true", "redactExecutionData": "false",
	} {
		if value := req.URL.Query().Get(field); value != want {
			t.Errorf("query %s = %q, want %q", field, value, want)
		}
	}

	got = f.run("execution", "list", "--output", "json")
	var page n8n.Page[n8n.Execution]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 || page.Data[0].ID != "1000" {
		t.Errorf("stdout = %q, want execution page (error: %v)", got.stdout, err)
	}
}

func TestExecutionListAllKeepsFiltersAndRejectsDetailedData(t *testing.T) {
	f := executionFixture(t, "")
	f.bodyFunc = func(page int) string {
		if page == 0 {
			return `{"data":[` + cliExecution + `],"nextCursor":"next"}`
		}
		return `{"data":[{"id":"1001","mode":"manual","status":"error","workflowId":"wf-1","finished":true}],"nextCursor":null}`
	}
	got := f.run("execution", "list", "--all", "--limit", "1", "--status", "error", "--workflow-id", "wf-1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.Execution]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 2 {
		t.Errorf("stdout = %q, want two executions (error: %v)", got.stdout, err)
	}
	second := f.lastRequest()
	if second.URL.Query().Get("cursor") != "next" || second.URL.Query().Get("status") != "error" || second.URL.Query().Get("workflowId") != "wf-1" {
		t.Errorf("second query = %q", second.URL.RawQuery)
	}

	before := f.requestCount()
	got = f.run("execution", "list", "--all", "--include-data")
	if got.code != ExitError || !strings.Contains(got.stderr, "cannot be combined") {
		t.Errorf("result = %+v", got)
	}
	if f.requestCount() != before {
		t.Error("unbounded detailed list reached instance")
	}
}

func TestExecutionListValidation(t *testing.T) {
	for name, args := range map[string][]string{
		"status":     {"execution", "list", "--status", "queued"},
		"timestamp":  {"execution", "list", "--started-after", "yesterday"},
		"range":      {"execution", "list", "--started-after", "2026-09-18T00:00:00Z", "--started-before", "2026-09-17T00:00:00Z"},
		"redaction":  {"execution", "list", "--redact-execution-data", "maybe"},
		"size limit": {"execution", "list", "--ignore-data-size-limit"},
	} {
		t.Run(name, func(t *testing.T) {
			f := executionFixture(t, `{"data":[],"nextCursor":null}`)
			before := f.requestCount()
			got := f.run(args...)
			if got.code != ExitError || got.stderr == "" {
				t.Errorf("result = %+v", got)
			}
			if f.requestCount() != before {
				t.Error("invalid list reached instance")
			}
		})
	}
}

func TestExecutionGet(t *testing.T) {
	f := executionFixture(t, cliExecution)
	got := f.run("execution", "get", "1000/one?x=1", "--include-data", "--redact-execution-data", "true")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if req := f.lastRequest(); req.URL.EscapedPath() != n8n.BasePath+"/executions/1000%2Fone%3Fx=1" || req.URL.Query().Get("includeData") != "true" || req.URL.Query().Get("redactExecutionData") != "true" {
		t.Errorf("request = %s?%s", req.URL.EscapedPath(), req.URL.RawQuery)
	}
	for _, want := range []string{"Execution:", "1000", "Workflow:", "wf-1", "Data included:", "true"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}

	got = f.run("execution", "get", "1000", "--output", "json")
	var execution n8n.Execution
	if err := json.Unmarshal([]byte(got.stdout), &execution); err != nil || execution.ID != "1000" {
		t.Errorf("stdout = %q, want execution (error: %v)", got.stdout, err)
	}
}

func TestExecutionDeleteConfirmationAndResult(t *testing.T) {
	f := executionFixture(t, cliDeletedExecution)
	before := f.requestCount()
	f.stdin = "no\n"
	got := f.run("execution", "delete", "1000")
	if got.code != ExitError || !strings.Contains(got.stderr, "cannot be undone") || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
		t.Errorf("declined result = %+v", got)
	}

	f.interactive = false
	got = f.run("execution", "delete", "1000")
	if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
		t.Errorf("non-interactive result = %+v", got)
	}

	got = f.run("execution", "delete", "1000/one?x=1", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodDelete || req.URL.EscapedPath() != n8n.BasePath+"/executions/1000%2Fone%3Fx=1" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	var deleted n8n.DeletedExecution
	if err := json.Unmarshal([]byte(got.stdout), &deleted); err != nil || deleted.ID.String() != "1000" {
		t.Errorf("stdout = %q, want deleted execution (error: %v)", got.stdout, err)
	}
}

func TestExecutionStopManyValidationConfirmationAndBody(t *testing.T) {
	t.Run("status required", func(t *testing.T) {
		f := executionFixture(t, `{"stopped":0}`)
		before := f.requestCount()
		got := f.run("execution", "stop-many", "--yes")
		if got.code != ExitError || !strings.Contains(got.stderr, "status") || f.requestCount() != before {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("declined", func(t *testing.T) {
		f := executionFixture(t, `{"stopped":0}`)
		before := f.requestCount()
		f.stdin = "no\n"
		got := f.run("execution", "stop-many", "--status", "running")
		if got.code != ExitError || !strings.Contains(got.stderr, "every accessible workflow") || f.requestCount() != before {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("stopped", func(t *testing.T) {
		f := executionFixture(t, `{"stopped":2}`)
		f.interactive = false
		got := f.run("execution", "stop-many", "--status", "queued", "--status", "running", "--workflow-id", "wf-1",
			"--started-after", "2026-09-17T00:00:00Z", "--started-before", "2026-09-18T00:00:00Z", "--yes", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+"/executions/stop" {
			t.Errorf("request = %s %s", req.Method, req.URL.Path)
		}
		assertCLIJSON(t, `{"status":["queued","running"],"workflowId":"wf-1","startedAfter":"2026-09-17T00:00:00Z","startedBefore":"2026-09-18T00:00:00Z"}`, f.lastBody())
		var result n8n.StopManyExecutionsResult
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Stopped != 2 {
			t.Errorf("stdout = %q (error: %v)", got.stdout, err)
		}
	})
}

func TestExecutionStopAndRetry(t *testing.T) {
	t.Run("stop", func(t *testing.T) {
		f := executionFixture(t, `{"mode":"manual","startedAt":"2026-09-17T10:00:01Z","stoppedAt":"2026-09-17T10:00:02Z","finished":false,"status":"canceled"}`)
		f.interactive = false
		got := f.run("execution", "stop", "1000/one?x=1", "--yes")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+"/executions/1000%2Fone%3Fx=1/stop" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		if !strings.Contains(got.stdout, "canceled") {
			t.Errorf("stdout = %q", got.stdout)
		}
	})

	t.Run("retry", func(t *testing.T) {
		f := executionFixture(t, cliExecution)
		got := f.run("execution", "retry", "1000/one?x=1", "--latest-workflow", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+"/executions/1000%2Fone%3Fx=1/retry" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"loadWorkflow":true}`, f.lastBody())
		var execution n8n.Execution
		if err := json.Unmarshal([]byte(got.stdout), &execution); err != nil || execution.ID != "1000" {
			t.Errorf("stdout = %q (error: %v)", got.stdout, err)
		}
	})
}

func TestExecutionTags(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		f := executionFixture(t, `[{"id":"tag-1","name":"Production","createdAt":"2026-09-17T00:00:00Z","updatedAt":"2026-09-17T00:00:00Z"}]`)
		got := f.run("execution", "tag", "list", "1000/one?x=1")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if f.lastRequest().URL.EscapedPath() != n8n.BasePath+"/executions/1000%2Fone%3Fx=1/tags" || !strings.Contains(got.stdout, "Production") {
			t.Errorf("request/output = %s / %s", f.lastRequest().URL.EscapedPath(), got.stdout)
		}
	})

	t.Run("set and clear", func(t *testing.T) {
		f := executionFixture(t, `[{"id":"tag-1","name":"Production"}]`)
		got := f.run("execution", "tag", "set", "1000", "--tag-id", "tag-1", "--tag-id", "tag-2", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if f.lastRequest().Method != http.MethodPut {
			t.Errorf("method = %s", f.lastRequest().Method)
		}
		assertCLIJSON(t, `[{"id":"tag-1"},{"id":"tag-2"}]`, f.lastBody())

		f.body = `[]`
		got = f.run("execution", "tag", "set", "1000", "--clear", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("clear exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `[]`, f.lastBody())
		if strings.TrimSpace(got.stdout) != "[]" {
			t.Errorf("stdout = %q, want []", got.stdout)
		}
	})

	t.Run("set validates choice", func(t *testing.T) {
		f := executionFixture(t, `[]`)
		for _, args := range [][]string{
			{"execution", "tag", "set", "1000"},
			{"execution", "tag", "set", "1000", "--clear", "--tag-id", "tag-1"},
		} {
			before := f.requestCount()
			got := f.run(args...)
			if got.code != ExitError || f.requestCount() != before {
				t.Errorf("result = %+v", got)
			}
		}
	})
}

func TestExecutionAPIErrors(t *testing.T) {
	f := executionFixture(t, cliExecution)
	f.status = http.StatusNotFound
	got := f.run("execution", "get", "missing")
	if got.code != ExitError || !strings.Contains(got.stderr, "no execution") || !strings.Contains(got.stderr, "execution list") {
		t.Errorf("result = %+v", got)
	}

	f.status = http.StatusForbidden
	got = f.run("execution", "list")
	if got.code != ExitError || !strings.Contains(got.stderr, "discover --resource execution") {
		t.Errorf("result = %+v", got)
	}
}
