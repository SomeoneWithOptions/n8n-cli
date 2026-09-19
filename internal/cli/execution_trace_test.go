package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// traceExecution builds an execution with the given status and run data.
func traceExecution(id, status string, finished bool, runData string) string {
	stoppedAt := "null"
	if finished {
		stoppedAt = `"2026-09-15T22:27:19.511Z"`
	}
	data := "null"
	if runData != "" {
		data = `{"version":1,"resultData":{"runData":{` + runData + `},"lastNodeExecuted":"x"}}`
	}
	return `{"id":"` + id + `","finished":` + boolText(finished) + `,"mode":"manual","status":"` + status +
		`","workflowId":"wf-1","startedAt":"2026-09-15T22:27:14.371Z","stoppedAt":` + stoppedAt + `,"data":` + data + `}`
}

const (
	traceNodeTrigger = `"Slack Trigger1":[{"startTime":1757975234371,"executionIndex":0,"executionTime":0,"executionStatus":"success","data":{"main":[[{"json":{"channel":"deals"}}]]}}]`
	traceNodeCheck   = `"Is New Deal Post":[{"startTime":1757975234371,"executionIndex":1,"executionTime":11,"executionStatus":"success"}]`
	traceNodeFailed  = `"Get Rotation1":[{"startTime":1757975235024,"executionIndex":2,"executionTime":4442,"executionStatus":"error","error":{"name":"NodeOperationError","message":"Sheet with name Rotation not found","node":{"name":"Get Rotation1"}}}]`
)

var traceFailedRun = traceExecution("163", "error", true, strings.Join([]string{traceNodeTrigger, traceNodeCheck, traceNodeFailed}, ","))

// traceFixture routes the unfiltered list, the status=running list (empty
// when running is nil), one execution by ID and the workflow. Only unfiltered
// list requests are counted.
func traceFixture(t *testing.T, workflowBody string, execution func(n int) string, list func(n int) string, running func(n int) string) (*fixture, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	f := newFixture(t)
	f.login()
	var gets, lists atomic.Int32
	f.route = func(r *http.Request, _ int) (int, string) {
		switch {
		case r.URL.Path == n8n.BasePath+n8n.ExecutionsPath && r.URL.Query().Get("status") == "running":
			if running == nil {
				return http.StatusOK, pageOf()
			}
			return http.StatusOK, running(int(lists.Load()))
		case r.URL.Path == n8n.BasePath+n8n.ExecutionsPath:
			return http.StatusOK, list(int(lists.Add(1)))
		case strings.HasPrefix(r.URL.Path, n8n.BasePath+n8n.ExecutionsPath+"/"):
			if r.URL.Query().Get("includeData") != "true" {
				t.Errorf("trace read %q without includeData=true", r.URL.RawQuery)
			}
			return http.StatusOK, execution(int(gets.Add(1)))
		case strings.HasPrefix(r.URL.Path, n8n.BasePath+n8n.WorkflowsPath+"/"):
			if workflowBody == "" {
				return http.StatusForbidden, `{"message":"forbidden"}`
			}
			return http.StatusOK, workflowBody
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			return http.StatusNotFound, `{"message":"Not Found"}`
		}
	}
	return f, &gets, &lists
}

func fixed(body string) func(int) string { return func(int) string { return body } }

func TestExecutionTraceOneShotText(t *testing.T) {
	f, _, _ := traceFixture(t, watchWorkflow, fixed(traceFailedRun), nil, nil)
	got := f.run("execution", "trace", "163")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, want 0 for a printed failed run (stderr: %s)", got.code, got.stderr)
	}
	for _, want := range []string{
		"Workflow:  Deal Desk (wf-1)",
		"Execution: 163 [error] | Mode: manual | Duration: 5140ms",
		"Started:   2026-09-15T22:27:14.371Z",
		"Stopped:   2026-09-15T22:27:19.511Z",
		"NODE EXECUTION TRACE:",
		" [ok]   Slack Trigger1           (0ms)",
		" [ok]   Is New Deal Post         (11ms)",
		" [fail] Get Rotation1            (4442ms)",
		"        Error: Sheet with name Rotation not found",
		"        Type:  NodeOperationError",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, "Data:") || strings.Contains(got.stdout, "deals") {
		t.Error("node data printed without --verbose")
	}
	if strings.Contains(got.stdout, "Run error") {
		t.Error("run error repeated although a node already carries it")
	}
	trigger := strings.Index(got.stdout, "Slack Trigger1")
	failed := strings.Index(got.stdout, "Get Rotation1")
	if trigger < 0 || failed < trigger {
		t.Error("nodes are not in execution order")
	}
	for _, r := range got.stdout {
		if r > 127 {
			t.Fatalf("stdout contains non-ASCII %q", r)
		}
	}

	got = f.run("execution", "trace", "163", "--verbose")
	if !strings.Contains(got.stdout, "Data:") || !strings.Contains(got.stdout, `"channel": "deals"`) {
		t.Errorf("--verbose stdout missing indented node data:\n%s", got.stdout)
	}

	got = f.run("execution", "trace", "163", "--fail-on-error")
	if got.code != ExitError || !strings.Contains(got.stderr, "execution 163 finished with status error") {
		t.Errorf("--fail-on-error result = %+v, want exit 1 with the reason", got)
	}
	if !strings.Contains(got.stdout, "Get Rotation1") || strings.Contains(got.stderr, "--help") {
		t.Error("--fail-on-error must still print the trace and must not print the usage hint")
	}
}

func TestExecutionTraceOneShotJSONAndUnresolvedWorkflow(t *testing.T) {
	f, _, _ := traceFixture(t, "", fixed(traceFailedRun), nil, nil)
	got := f.run("execution", "trace", "163", "--output", "json", "--verbose")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d (stderr: %s)", got.code, got.stderr)
	}
	var doc traceDocument
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatalf("stdout is not a JSON object: %v\n%s", err, got.stdout)
	}
	if doc.SchemaVersion != 1 || doc.Execution.ID != "163" || doc.Execution.Status != "error" || !doc.Execution.Finished {
		t.Errorf("document = %+v", doc.Execution)
	}
	if doc.Execution.WorkflowName != nil {
		t.Errorf("workflowName = %q, want null when workflow:read is denied", *doc.Execution.WorkflowName)
	}
	if doc.Execution.DurationMs == nil || *doc.Execution.DurationMs != 5140 || !doc.Data.Included {
		t.Errorf("duration/data = %v/%+v", doc.Execution.DurationMs, doc.Data)
	}
	if len(doc.Nodes) != 3 || doc.Nodes[2].Name != "Get Rotation1" || doc.Nodes[2].Error == nil || doc.Nodes[2].Error.Name != "NodeOperationError" {
		t.Errorf("nodes = %+v", doc.Nodes)
	}
	if doc.Nodes[0].StartedAt != "2025-09-15T22:27:14.371Z" || len(doc.Nodes[0].Data) == 0 || doc.Nodes[0].Type != "" {
		t.Errorf("first node = %+v, want RFC3339 start, verbose data and no stream type", doc.Nodes[0])
	}
	if doc.Error == nil || doc.Error.Message != "Sheet with name Rotation not found" {
		t.Errorf("error = %+v", doc.Error)
	}
	if !strings.HasPrefix(got.stdout, "{\n  ") {
		t.Error("one-shot JSON must be indented")
	}
}

func TestExecutionTraceWorkflowIDPicksMostRecentRun(t *testing.T) {
	f, gets, _ := traceFixture(t, watchWorkflow, fixed(traceFailedRun), fixed(pageOf(watchExecution("163", "error", "wf-1", true), watchExecution("100", "success", "wf-1", true))), nil)
	got := f.run("execution", "trace", "--workflow-id", "wf-1")
	if got.code != ExitSuccess || !strings.Contains(got.stdout, "Execution: 163") {
		t.Fatalf("result = %+v", got)
	}
	list := f.requests[1]
	if list.URL.Query().Get("workflowId") != "wf-1" || list.URL.Query().Get("limit") != "5" {
		t.Errorf("list query = %q, want workflowId=wf-1&limit=5", list.URL.RawQuery)
	}
	if gets.Load() != 1 || !strings.HasSuffix(f.requests[3].URL.Path, "/executions/163") {
		t.Errorf("expected exactly one read of execution 163; requests: %d", gets.Load())
	}

	f, _, _ = traceFixture(t, watchWorkflow, fixed(traceFailedRun), fixed(pageOf()), nil)
	got = f.run("execution", "trace", "--workflow-id", "wf-1")
	if got.code != ExitError || !strings.Contains(got.stderr, "has no executions for workflow") {
		t.Errorf("empty workflow result = %+v", got)
	}
}

func TestExecutionTraceValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"both selectors":   {[]string{"163", "--workflow-id", "wf-1"}, "not both"},
		"neither selector": {[]string{}, "execution ID or --workflow-id is required"},
		"interval":         {[]string{"163", "--follow", "--interval", "100ms"}, "at least 1s"},
		"redaction":        {[]string{"163", "--redact-execution-data", "maybe"}, "unknown --redact-execution-data"},
		"output":           {[]string{"163", "--output", "table"}, "unknown output format"},
		"whitespace":       {[]string{" 163"}, "whitespace"},
	} {
		t.Run(name, func(t *testing.T) {
			f := executionFixture(t, traceFailedRun)
			got := f.run(append([]string{"execution", "trace"}, tc.args...)...)
			if got.code != ExitError || !strings.Contains(got.stderr, tc.want) {
				t.Errorf("result = %+v, want exit 1 mentioning %q", got, tc.want)
			}
			if f.requestCount() != 1 {
				t.Error("invalid trace reached the instance")
			}
		})
	}

	f := executionFixture(t, "")
	f.status = http.StatusNotFound
	got := f.run("execution", "trace", "999")
	if got.code != ExitError || !strings.Contains(got.stderr, `has no execution "999" (404)`) {
		t.Errorf("not found result = %+v", got)
	}
}

func TestExecutionTraceExplainsMissingData(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"too large": {`{"id":"7","finished":true,"status":"success","workflowId":"wf-1","dataTooLargeToDisplay":true}`, "exceeds the instance display-size limit"},
		"running":   {traceExecution("7", "running", false, ""), "is still running and has no stored node data yet"},
		"no data":   {traceExecution("7", "success", true, ""), "has no stored run data"},
		"no nodes":  {`{"id":"7","finished":true,"status":"success","workflowId":"wf-1","data":{"resultData":{"runData":{}}}}`, "without running any node"},
	} {
		t.Run(name, func(t *testing.T) {
			f, _, _ := traceFixture(t, watchWorkflow, fixed(tc.body), nil, nil)
			got := f.run("execution", "trace", "7")
			if got.code != ExitSuccess || !strings.Contains(got.stdout, tc.want) {
				t.Errorf("result = %+v, want %q on stdout", got, tc.want)
			}
			got = f.run("execution", "trace", "7", "--output", "json")
			var doc traceDocument
			if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || !strings.Contains(doc.Data.Notice, tc.want) || len(doc.Nodes) != 0 {
				t.Errorf("json = %+v (err %v), want the notice and an empty nodes array", doc, err)
			}
		})
	}
}

func TestExecutionTraceFollowStreamsNodesOnceAndWarnsOnProgressOff(t *testing.T) {
	autoTicks(t)
	stages := []string{
		traceExecution("163", "running", false, traceNodeTrigger),
		traceExecution("163", "running", false, strings.Join([]string{traceNodeTrigger, traceNodeCheck}, ",")),
		traceFailedRun,
	}
	f, gets, _ := traceFixture(t, `{"id":"wf-1","name":"Deal Desk","settings":{"saveExecutionProgress":false},"nodes":[]}`, func(n int) string {
		if n > len(stages) {
			t.Error("polled after the run finished")
			n = len(stages)
		}
		return stages[n-1]
	}, nil, nil)
	got := f.run("execution", "trace", "163", "--follow")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d (stderr: %s)", got.code, got.stderr)
	}
	if gets.Load() != 3 {
		t.Errorf("execution read %d times, want 3 (one per stage, then stop)", gets.Load())
	}
	for _, node := range []string{"Slack Trigger1", "Is New Deal Post", "Get Rotation1"} {
		if n := strings.Count(got.stdout, node); n != 1 {
			t.Errorf("%q printed %d times, want once:\n%s", node, n, got.stdout)
		}
	}
	for _, want := range []string{"Execution: 163 [running] | Mode: manual\n", "Result:    error | Duration: 5140ms | Stopped: 2026-09-15T22:27:19.511Z"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if !strings.Contains(got.stderr, `workflow "Deal Desk" has saveExecutionProgress disabled`) || !strings.Contains(got.stderr, "Save execution progress") {
		t.Errorf("stderr = %q, want the progress-saving warning", got.stderr)
	}
	if strings.Contains(got.stdout, "Warning") {
		t.Error("warning leaked to stdout")
	}
}

func TestExecutionTraceFollowWorkflowAttachesOrWaitsNDJSON(t *testing.T) {
	autoTicks(t)
	pages := []string{
		pageOf(watchExecution("163", "error", "wf-1", true)), // finished: becomes the baseline
		pageOf(watchExecution("163", "error", "wf-1", true)), // nothing new yet
		pageOf(watchExecution("163", "error", "wf-1", true)), // the next run is only in the running list
	}
	runningPages := map[int]string{3: pageOf(watchExecution("164", "running", "wf-1", false))}
	stages := []string{
		traceExecution("164", "running", false, traceNodeTrigger),
		traceExecution("164", "success", true, strings.Join([]string{traceNodeTrigger, traceNodeCheck}, ",")),
	}
	f, gets, lists := traceFixture(t, `{"id":"wf-1","name":"Deal Desk","settings":{}}`,
		func(n int) string { return stages[min(n, len(stages))-1] },
		func(n int) string { return pages[min(n, len(pages))-1] },
		func(n int) string {
			if page, ok := runningPages[n]; ok {
				return page
			}
			return pageOf()
		})
	got := f.run("execution", "trace", "--workflow-id", "wf-1", "--follow", "--output", "json", "--fail-on-error")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d (stderr: %s)", got.code, got.stderr)
	}
	if lists.Load() != 3 || gets.Load() != 2 {
		t.Errorf("lists = %d, gets = %d; want 3 list polls then 2 reads", lists.Load(), gets.Load())
	}
	if !strings.Contains(got.stderr, `Waiting for a run of Deal Desk (wf-1)...`) {
		t.Errorf("stderr = %q, want the waiting notice", got.stderr)
	}
	if !strings.Contains(got.stderr, "inherits the instance default") {
		t.Errorf("stderr = %q, want the unknown-progress warning", got.stderr)
	}
	lines := strings.Split(strings.TrimSpace(got.stdout), "\n")
	types := []string{}
	for i, line := range lines {
		var rec struct {
			SchemaVersion int    `json:"schemaVersion"`
			Type          string `json:"type"`
			ExecutionID   string `json:"executionId"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil || rec.SchemaVersion != 1 || rec.ExecutionID != "164" {
			t.Fatalf("line %d invalid (%v): %s", i, err, line)
		}
		types = append(types, rec.Type)
	}
	if got, want := strings.Join(types, ","), "header,node,node,result"; got != want {
		t.Errorf("record types = %s, want %s\n%s", got, want, strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[0], `"saveExecutionProgress":"unknown"`) || !strings.Contains(lines[0], `"workflowName":"Deal Desk"`) {
		t.Errorf("header = %s", lines[0])
	}
	if !strings.Contains(lines[3], `"status":"success"`) || !strings.Contains(lines[3], `"durationMs":5140`) {
		t.Errorf("result = %s", lines[3])
	}
}

func TestExecutionTraceFollowAttachesToActiveRunAndFailsOnError(t *testing.T) {
	autoTicks(t)
	f, gets, lists := traceFixture(t, "", fixed(traceFailedRun),
		fixed(pageOf(watchExecution("162", "error", "wf-1", true))),
		fixed(pageOf(watchExecution("163", "running", "wf-1", false))))
	got := f.run("execution", "trace", "--workflow-id", "wf-1", "--follow", "--fail-on-error")
	if got.code != ExitError || !strings.Contains(got.stderr, "finished with status error") {
		t.Errorf("result = %+v, want exit 1 for the failed run", got)
	}
	if lists.Load() != 1 || gets.Load() != 1 {
		t.Errorf("lists = %d, gets = %d; want an immediate attach", lists.Load(), gets.Load())
	}
	if !strings.Contains(got.stderr, "could not read workflow wf-1") {
		t.Errorf("stderr = %q, want the failed preflight reported, not fatal", got.stderr)
	}
	if !strings.Contains(got.stdout, "[fail] Get Rotation1") {
		t.Errorf("stdout missing the trace:\n%s", got.stdout)
	}
	if strings.Contains(got.stderr, "Waiting") {
		t.Error("an active run must be attached to, not waited for")
	}
}

func TestExecutionTraceFollowNoPreflightAndCancelWhileWaiting(t *testing.T) {
	autoTicks(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f, gets, _ := traceFixture(t, "", fixed(traceFailedRun), func(n int) string {
		if n == 2 {
			cancel()
		}
		return pageOf(watchExecution("163", "error", "wf-1", true))
	}, nil)
	got := f.runContext(ctx, "execution", "trace", "--workflow-id", "wf-1", "--follow", "--no-preflight")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, want 0 on Ctrl+C while waiting (stderr: %s)", got.code, got.stderr)
	}
	if gets.Load() != 0 || got.stdout != "" {
		t.Errorf("nothing should be traced; gets = %d, stdout = %q", gets.Load(), got.stdout)
	}
	if strings.Contains(got.stderr, "canceled") || strings.Contains(got.stderr, "saveExecutionProgress") {
		t.Errorf("stderr = %q, want no cancel diagnostic and no preflight warning", got.stderr)
	}
	if !strings.Contains(got.stderr, "Waiting for a run of wf-1...") {
		t.Errorf("stderr = %q, want the waiting notice with the bare ID when the name cannot be read", got.stderr)
	}
}

func TestNodeStatusMarkerStaysNarrow(t *testing.T) {
	for status, want := range map[string]string{
		"success": "[ok]", "error": "[fail]", "crashed": "[fail]", "waiting": "[wait]",
		"running": "[run]", "new": "[run]", "canceled": "[stop]", "": "[?]", "unknown": "[unkn]",
	} {
		got := nodeStatusMarker(status)
		if got != want || len(got) > 6 {
			t.Errorf("nodeStatusMarker(%q) = %q, want %q within 6 characters", status, got, want)
		}
	}
}
