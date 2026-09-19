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

// watchExecution renders one metadata-only execution row for a staged page.
func watchExecution(id, status, workflowID string, stopped bool) string {
	stoppedAt := "null"
	if stopped {
		stoppedAt = `"2026-09-18T17:14:00.195Z"`
	}
	return `{"id":"` + id + `","finished":` + boolText(stopped) + `,"mode":"webhook","status":"` + status +
		`","workflowId":"` + workflowID + `","startedAt":"2026-09-18T17:14:00.030Z","stoppedAt":` + stoppedAt + `}`
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func pageOf(rows ...string) string {
	return `{"data":[` + strings.Join(rows, ",") + `],"nextCursor":null}`
}

const watchWorkflow = `{"id":"wf-1","name":"Deal Desk","active":true,"nodes":[{"name":"a"},{"name":"b"}],"connections":{},"settings":{}}`

// watchFixture stages a logged-in instance whose unfiltered list endpoint
// answers from pages, in order, whose status=running list answers running
// every time, and whose workflow endpoint names wf-1 and denies every other
// workflow. cancelAt cancels the run's context when that many unfiltered list
// requests have been made; the response to that request is lost to the
// cancellation, so stage one page more than the test wants rendered.
func watchFixture(t *testing.T, cancel context.CancelFunc, cancelAt int, running string, pages ...string) (*fixture, *atomic.Int32) {
	t.Helper()
	f := newFixture(t)
	f.login()
	var lists atomic.Int32
	f.route = func(r *http.Request, _ int) (int, string) {
		switch {
		case r.URL.Path == n8n.BasePath+n8n.ExecutionsPath && r.URL.Query().Get("status") == "running":
			return http.StatusOK, running
		case r.URL.Path == n8n.BasePath+n8n.ExecutionsPath:
			n := int(lists.Add(1))
			if n >= cancelAt && cancel != nil {
				cancel()
			}
			if n > len(pages) {
				n = len(pages)
			}
			return http.StatusOK, pages[n-1]
		case r.URL.Path == n8n.BasePath+n8n.WorkflowsPath+"/wf-1":
			return http.StatusOK, watchWorkflow
		case strings.HasPrefix(r.URL.Path, n8n.BasePath+n8n.WorkflowsPath+"/"):
			return http.StatusForbidden, `{"message":"forbidden"}`
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			return http.StatusNotFound, `{"message":"Not Found"}`
		}
	}
	return f, &lists
}

func TestExecutionWatchDashboardWithoutTerminalHasNoEscapeCodes(t *testing.T) {
	autoTicks(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	page := pageOf(watchExecution("254", "success", "wf-1", true), watchExecution("163", "error", "wf-2", true))
	f, _ := watchFixture(t, cancel, 2, pageOf(), page, page)

	got := f.runContext(ctx, "execution", "watch", "--limit", "5", "--interval", "1s")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, want 0 on cancellation (stderr: %s)", got.code, got.stderr)
	}
	if strings.Contains(got.stdout, "\033") {
		t.Errorf("stdout carries escape codes although it is not a terminal:\n%s", got.stdout)
	}
	for _, want := range []string{"Instance:", "Watching:", "every 1s", "dashboard", "ID", "STATUS", "MODE", "STARTED", "DURATION", "WORKFLOW", "254", "success", "165ms", "Deal Desk", "wf-2"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	for _, unwanted := range []string{"NODES", "ERROR", "\033"} {
		if strings.Contains(got.stdout, unwanted) {
			t.Errorf("stdout has %q without --nodes:\n%s", unwanted, got.stdout)
		}
	}
	if !strings.Contains(got.stdout, "T") || !strings.Contains(got.stdout, "Z\n") {
		t.Errorf("stdout has no RFC3339 timestamp line:\n%s", got.stdout)
	}
	if strings.Contains(got.stderr, "canceled") {
		t.Errorf("stderr = %q, want no cancellation diagnostic", got.stderr)
	}
	req := f.lastRequest()
	if req.URL.Query().Get("limit") != "5" || req.URL.Query().Get("includeData") != "" {
		t.Errorf("list query = %q, want limit=5 and no includeData", req.URL.RawQuery)
	}
}

func TestExecutionWatchFollowPrintsEachChangeOnce(t *testing.T) {
	autoTicks(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p0 := pageOf(watchExecution("254", "success", "wf-1", true), watchExecution("163", "error", "wf-1", true))
	p1 := pageOf(watchExecution("255", "running", "wf-1", false), watchExecution("254", "success", "wf-1", true), watchExecution("163", "error", "wf-1", true))
	p2 := pageOf(watchExecution("255", "success", "wf-1", true), watchExecution("254", "success", "wf-1", true), watchExecution("163", "error", "wf-1", true))
	f, _ := watchFixture(t, cancel, 4, pageOf(), p0, p1, p2, p2)

	got := f.runContext(ctx, "execution", "watch", "--follow", "--workflow-id", "wf-1", "--interval", "1s")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d (stderr: %s)", got.code, got.stderr)
	}
	lines := strings.Split(strings.TrimSpace(got.stdout), "\n")
	if len(lines) != 5 {
		t.Fatalf("stdout has %d lines, want header + 4 rows:\n%s", len(lines), got.stdout)
	}
	if !strings.HasPrefix(lines[0], "ID") || strings.Contains(lines[0], "WORKFLOW") {
		t.Errorf("header = %q, want columns without WORKFLOW when one workflow is selected", lines[0])
	}
	wantOrder := []string{"163 error", "254 success", "255 running", "255 success"}
	for i, want := range wantOrder {
		fields := strings.Fields(lines[i+1])
		if len(fields) < 2 || fields[0]+" "+fields[1] != want {
			t.Errorf("line %d = %q, want it to start with %q", i+1, lines[i+1], want)
		}
	}
	if strings.Count(got.stdout, "254 ") != 1 {
		t.Errorf("execution 254 printed %d times, want once:\n%s", strings.Count(got.stdout, "254 "), got.stdout)
	}
	for _, want := range []string{"Instance:", "Workflow:", "Deal Desk (wf-1)", "follow"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr missing %q (context lines belong on stderr in follow mode):\n%s", want, got.stderr)
		}
	}
	if strings.Contains(got.stdout, "Instance:") {
		t.Error("follow mode wrote context lines to stdout")
	}
}

func TestExecutionWatchNDJSONIsPureAndVersioned(t *testing.T) {
	autoTicks(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p0 := pageOf(watchExecution("254", "success", "wf-1", true))
	p1 := pageOf(watchExecution("255", "running", "wf-1", false), watchExecution("254", "success", "wf-1", true))
	f, _ := watchFixture(t, cancel, 3, pageOf(), p0, p1, p1)

	got := f.runContext(ctx, "execution", "watch", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d (stderr: %s)", got.code, got.stderr)
	}
	lines := strings.Split(strings.TrimSpace(got.stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout has %d lines, want 2:\n%s", len(lines), got.stdout)
	}
	for i, line := range lines {
		var rec watchRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("line %d is not JSON: %v\n%s", i, err, line)
		}
		if rec.SchemaVersion != 1 || rec.Type != "execution" || rec.WorkflowName == nil || *rec.WorkflowName != "Deal Desk" {
			t.Errorf("line %d = %+v, want schemaVersion 1, type execution and the resolved name", i, rec)
		}
		if rec.Nodes != nil || rec.Error != nil {
			t.Errorf("line %d carries nodes/error without --nodes: %s", i, line)
		}
		if strings.Contains(line, "\n") || strings.Contains(line, "  ") {
			t.Errorf("line %d is not compact: %q", i, line)
		}
	}
	var second watchRecord
	_ = json.Unmarshal([]byte(lines[1]), &second)
	if second.ID != "255" || second.Status != "running" || second.DurationMs != nil || second.StoppedAt != nil {
		t.Errorf("second record = %+v, want the new running execution with null duration", second)
	}
	var first watchRecord
	_ = json.Unmarshal([]byte(lines[0]), &first)
	if first.DurationMs == nil || *first.DurationMs != 165 {
		t.Errorf("first record duration = %v, want 165", first.DurationMs)
	}
	if strings.Contains(got.stdout, "Instance:") {
		t.Error("JSON mode wrote text to stdout")
	}
}

func TestExecutionWatchNodesFetchesFinishedRunOnce(t *testing.T) {
	autoTicks(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newFixture(t)
	f.login()
	var lists, details atomic.Int32
	detail := `{"id":"163","finished":true,"mode":"manual","status":"error","workflowId":"wf-1",
	  "startedAt":"2026-09-15T22:27:14.371Z","stoppedAt":"2026-09-15T22:27:19.511Z",
	  "workflowData":{"nodes":[{"name":"a"},{"name":"b"},{"name":"c"}]},
	  "data":{"resultData":{"runData":{
	    "a":[{"executionIndex":0,"executionTime":1,"executionStatus":"success"}],
	    "b":[{"executionIndex":1,"executionTime":2,"executionStatus":"error","error":{"name":"NodeOperationError","message":"Sheet with name\n Rotation not found"}}]}}}}`
	page := pageOf(`{"id":"163","finished":true,"mode":"manual","status":"error","workflowId":"wf-1","startedAt":"2026-09-15T22:27:14.371Z","stoppedAt":"2026-09-15T22:27:19.511Z"}`)
	f.route = func(r *http.Request, _ int) (int, string) {
		switch r.URL.Path {
		case n8n.BasePath + n8n.ExecutionsPath:
			if r.URL.Query().Get("status") == "running" {
				return http.StatusOK, pageOf()
			}
			if lists.Add(1) >= 3 {
				cancel()
			}
			return http.StatusOK, page
		case n8n.BasePath + n8n.ExecutionsPath + "/163":
			details.Add(1)
			if r.URL.Query().Get("includeData") != "true" {
				t.Errorf("detail query = %q, want includeData=true", r.URL.RawQuery)
			}
			return http.StatusOK, detail
		default:
			return http.StatusOK, watchWorkflow
		}
	}
	got := f.runContext(ctx, "execution", "watch", "--nodes", "--workflow-id", "wf-1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d (stderr: %s)", got.code, got.stderr)
	}
	for _, want := range []string{"NODES", "ERROR", "2/3", "Sheet with name Rotation not found"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if details.Load() != 1 {
		t.Errorf("finished execution fetched %d times across %d ticks, want once", details.Load(), lists.Load())
	}
	if req := f.requests[0]; req.URL.Query().Get("includeData") != "" {
		t.Error("the list poll requested detailed data; --nodes must fetch per execution")
	}
}

func TestExecutionWatchValidationAndFatalErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"interval too short": {[]string{"--interval", "500ms"}, "at least 1s"},
		"interval too long":  {[]string{"--interval", "2h"}, "at most 1h"},
		"limit zero":         {[]string{"--limit", "0"}, "between 1 and 250"},
		"limit too big":      {[]string{"--limit", "251"}, "between 1 and 250"},
		"status":             {[]string{"--status", "queued"}, "unknown execution status"},
		"output":             {[]string{"--output", "yaml"}, "unknown output format"},
	} {
		t.Run(name, func(t *testing.T) {
			f := executionFixture(t, pageOf())
			got := f.run(append([]string{"execution", "watch"}, tc.args...)...)
			if got.code != ExitError || !strings.Contains(got.stderr, tc.want) {
				t.Errorf("result = %+v, want exit 1 mentioning %q", got, tc.want)
			}
			if f.requestCount() != 1 { // only the login validation
				t.Error("invalid watch reached the instance")
			}
		})
	}

	// A rejected credential on the first poll is an error, not a retry loop.
	f := executionFixture(t, "")
	f.status = http.StatusUnauthorized
	got := f.run("execution", "watch")
	if got.code != ExitError || !strings.Contains(got.stderr, "401") {
		t.Errorf("result = %+v, want exit 1 with the 401 explained", got)
	}
}

func TestExecutionWatchRetriesTransientPollFailure(t *testing.T) {
	autoTicks(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newFixture(t)
	f.login()
	var lists atomic.Int32
	f.route = func(r *http.Request, _ int) (int, string) {
		if r.URL.Path != n8n.BasePath+n8n.ExecutionsPath {
			return http.StatusOK, watchWorkflow
		}
		if r.URL.Query().Get("status") == "running" {
			return http.StatusOK, pageOf()
		}
		switch lists.Add(1) {
		case 1:
			return http.StatusOK, pageOf(watchExecution("1", "success", "wf-1", true))
		case 2:
			return http.StatusBadGateway, `<html>upstream down</html>`
		case 3:
			return http.StatusOK, pageOf(watchExecution("2", "success", "wf-1", true), watchExecution("1", "success", "wf-1", true))
		default:
			cancel()
			return http.StatusOK, pageOf()
		}
	}
	got := f.runContext(ctx, "execution", "watch", "--follow", "--workflow-id", "wf-1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d (stderr: %s)", got.code, got.stderr)
	}
	if !strings.Contains(got.stderr, "Warning: poll failed, retrying in 2s") || !strings.Contains(got.stderr, "502") {
		t.Errorf("stderr = %q, want the transient failure reported", got.stderr)
	}
	lines := strings.Split(strings.TrimSpace(got.stdout), "\n")
	if len(lines) != 3 || strings.Fields(lines[1])[0] != "1" || strings.Fields(lines[2])[0] != "2" {
		t.Errorf("stdout = %q, want the header and both executions after the retry", got.stdout)
	}
}

// The unfiltered list omits running executions, so the watch merges a
// status=running query into every tick unless the user filtered by status.
func TestExecutionWatchMergesRunningExecutions(t *testing.T) {
	autoTicks(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := pageOf(watchExecution("254", "success", "wf-1", true))
	running := pageOf(`{"id":"255","finished":false,"mode":"webhook","status":"running","workflowId":"wf-1","startedAt":"2026-09-18T17:14:05.000Z","stoppedAt":null}`)
	f, _ := watchFixture(t, cancel, 2, running, finished, finished)

	got := f.runContext(ctx, "execution", "watch", "--workflow-id", "wf-1", "--limit", "5")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d (stderr: %s)", got.code, got.stderr)
	}
	lines := strings.Split(strings.TrimSpace(got.stdout), "\n")
	var rows []string
	for _, line := range lines {
		if fields := strings.Fields(line); len(fields) > 1 && (fields[0] == "255" || fields[0] == "254") {
			rows = append(rows, fields[0]+" "+fields[1]+" "+fields[4])
		}
	}
	if len(rows) < 2 || rows[0] != "255 running -" || rows[1] != "254 success 165ms" {
		t.Errorf("rows = %q, want the running execution first with no duration, then the finished one:\n%s", rows, got.stdout)
	}
	var runningQueries, plainQueries int
	for _, req := range f.requests {
		if req.URL.Path != n8n.BasePath+n8n.ExecutionsPath {
			continue
		}
		if req.URL.Query().Get("status") == "running" {
			runningQueries++
			if req.URL.Query().Get("workflowId") != "wf-1" || req.URL.Query().Get("limit") != "5" {
				t.Errorf("running query = %q, want the same filters as the plain one", req.URL.RawQuery)
			}
		} else {
			plainQueries++
		}
	}
	// The cancelled tick loses its plain response before the running query.
	if runningQueries == 0 || plainQueries-runningQueries > 1 {
		t.Errorf("running queries = %d, plain = %d; want one running query per completed tick", runningQueries, plainQueries)
	}

	// An explicit status filter is passed through untouched: no second query.
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	f, _ = watchFixture(t, cancel, 2, running, finished, finished)
	got = f.runContext(ctx, "execution", "watch", "--status", "success", "--follow")
	if got.code != ExitSuccess || strings.Contains(got.stdout, "255") {
		t.Errorf("result = %+v, want only the filtered executions", got)
	}
	for _, req := range f.requests {
		if req.URL.Path == n8n.BasePath+n8n.ExecutionsPath && req.URL.Query().Get("status") != "success" {
			t.Errorf("query %q sent although --status was given", req.URL.RawQuery)
		}
	}
}

// A waiting run carries a stoppedAt (when it went to sleep) that must not be
// read as a finished run.
func TestExecutionWatchWaitingRunHasNoDuration(t *testing.T) {
	autoTicks(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiting := pageOf(`{"id":"269","finished":false,"mode":"webhook","status":"waiting","workflowId":"wf-1","startedAt":"2026-09-19T00:09:16.422Z","stoppedAt":"2026-09-19T00:09:16.464Z"}`)
	f, _ := watchFixture(t, cancel, 2, pageOf(), waiting, waiting)
	got := f.runContext(ctx, "execution", "watch", "--workflow-id", "wf-1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d (stderr: %s)", got.code, got.stderr)
	}
	var rec watchRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(got.stdout)), &rec); err != nil {
		t.Fatalf("stdout = %q: %v", got.stdout, err)
	}
	if rec.Status != "waiting" || rec.DurationMs != nil || rec.StoppedAt != nil {
		t.Errorf("record = %+v, want waiting with null durationMs and stoppedAt", rec)
	}
}
