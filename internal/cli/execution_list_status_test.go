package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

func executionIDs(executions []n8n.Execution) []string {
	ids := make([]string, len(executions))
	for i, execution := range executions {
		ids[i] = execution.ID
	}
	return ids
}

func TestMergeExecutionsNewestFirst(t *testing.T) {
	success := []n8n.Execution{{ID: "10", Status: "success"}, {ID: "7", Status: "success"}, {ID: "3", Status: "success"}}
	errored := []n8n.Execution{{ID: "9", Status: "error"}, {ID: "7", Status: "error"}, {ID: "2", Status: "error"}}

	merged, truncated := mergeExecutionsNewestFirst(0, success, errored)
	if got, want := executionIDs(merged), []string{"10", "9", "7", "3", "2"}; !slices.Equal(got, want) || truncated {
		t.Errorf("merged = %v truncated %t, want %v untruncated", got, truncated, want)
	}
	if merged[2].Status != "success" {
		t.Errorf("duplicate 7 kept status %q, want the first list's success", merged[2].Status)
	}

	merged, truncated = mergeExecutionsNewestFirst(3, success, errored)
	if got, want := executionIDs(merged), []string{"10", "9", "7"}; !slices.Equal(got, want) || !truncated {
		t.Errorf("capped = %v truncated %t, want %v truncated", got, truncated, want)
	}
	if _, truncated = mergeExecutionsNewestFirst(5, success, errored); truncated {
		t.Error("five unique rows under a cap of five reported truncation")
	}

	merged, _ = mergeExecutionsNewestFirst(0, []n8n.Execution{{ID: "b"}, {ID: "9"}}, []n8n.Execution{{ID: "c"}})
	if got, want := executionIDs(merged), []string{"c", "b", "9"}; !slices.Equal(got, want) {
		t.Errorf("non-numeric merge = %v, want string order %v", got, want)
	}

	if merged, _ = mergeExecutionsNewestFirst(10); merged == nil {
		t.Error("empty merge is nil, want an empty slice so JSON stays []")
	}
}

func statusExecution(id, status string) string {
	return fmt.Sprintf(`{"id":%q,"status":%q,"workflowId":"wf-1","mode":"manual","finished":true}`, id, status)
}

// statusRoute answers GET /executions from per-status pages keyed by the
// cursor the request carries ("" for the first page).
func statusRoute(pages map[string]map[string]string) func(*http.Request, int) (int, string) {
	return func(r *http.Request, _ int) (int, string) {
		query := r.URL.Query()
		body, ok := pages[query.Get("status")][query.Get("cursor")]
		if !ok {
			return http.StatusBadRequest, `{"message":"unexpected page"}`
		}
		return http.StatusOK, body
	}
}

func TestExecutionListMultiStatusPage(t *testing.T) {
	pages := map[string]map[string]string{
		"success": {"": `{"data":[` + statusExecution("10", "success") + `,` + statusExecution("7", "success") + `],"nextCursor":"s2"}`},
		"error":   {"": `{"data":[` + statusExecution("9", "error") + `,` + statusExecution("2", "error") + `],"nextCursor":null}`},
	}
	for name, args := range map[string][]string{
		"comma":      {"--status", "success,error"},
		"repeated":   {"--status", "success", "--status", "error"},
		"whitespace": {"--status", "success, error"},
	} {
		t.Run(name, func(t *testing.T) {
			f := executionFixture(t, "")
			f.route = statusRoute(pages)
			before := f.requestCount()
			got := f.run(append([]string{"execution", "list", "--limit", "2", "--workflow-id", "wf-1",
				"--project-id", "pr-1", "--started-after", "2026-09-17T00:00:00Z", "--output", "json"}, args...)...)
			if got.code != ExitSuccess {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			if f.requestCount()-before != 2 {
				t.Fatalf("requests = %d, want one per status", f.requestCount()-before)
			}
			for i, status := range []string{"success", "error"} {
				query := f.request(before + i).URL.Query()
				for field, want := range map[string]string{
					"status": status, "limit": "2", "workflowId": "wf-1", "projectId": "pr-1",
					"startedAfter": "2026-09-17T00:00:00Z", "cursor": "",
				} {
					if value := query.Get(field); value != want {
						t.Errorf("request %d query %s = %q, want %q", i, field, value, want)
					}
				}
			}
			var page n8n.Page[n8n.Execution]
			if err := json.Unmarshal([]byte(got.stdout), &page); err != nil {
				t.Fatalf("stdout = %q: %v", got.stdout, err)
			}
			if ids := executionIDs(page.Data); !slices.Equal(ids, []string{"10", "9"}) || page.NextCursor != "" {
				t.Errorf("page = %v cursor %q, want [10 9] and no cursor", ids, page.NextCursor)
			}
			if !strings.Contains(got.stderr, "More executions match these statuses") {
				t.Errorf("stderr = %q, want the not-exhaustive hint", got.stderr)
			}
		})
	}
}

func TestExecutionListMultiStatusDefaultLimitAndText(t *testing.T) {
	f := executionFixture(t, "")
	f.route = statusRoute(map[string]map[string]string{
		"running": {"": `{"data":[` + statusExecution("12", "running") + `],"nextCursor":null}`},
		"error":   {"": `{"data":[` + statusExecution("9", "error") + `],"nextCursor":null}`},
	})
	before := f.requestCount()
	got := f.run("execution", "list", "--status", "running,error")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for i := before; i < f.requestCount(); i++ {
		if limit := f.request(i).URL.Query().Get("limit"); limit != "30" {
			t.Errorf("request %d limit = %q, want the multi-status default 30", i, limit)
		}
	}
	for _, want := range []string{"Executions:  2", "12", "running", "9", "error"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if strings.Index(got.stdout, "12") > strings.Index(got.stdout, "error") {
		t.Errorf("stdout not newest first:\n%s", got.stdout)
	}
	if strings.Contains(got.stdout, "Next cursor") || got.stderr != "" {
		t.Errorf("exhaustive merge printed a cursor or hint: stdout %q stderr %q", got.stdout, got.stderr)
	}
}

func TestExecutionListMultiStatusAll(t *testing.T) {
	f := executionFixture(t, "")
	f.route = statusRoute(map[string]map[string]string{
		"success": {
			"":   `{"data":[` + statusExecution("10", "success") + `],"nextCursor":"s2"}`,
			"s2": `{"data":[` + statusExecution("4", "success") + `],"nextCursor":null}`,
		},
		"error": {
			"":   `{"data":[` + statusExecution("9", "error") + `],"nextCursor":"e2"}`,
			"e2": `{"data":[` + statusExecution("6", "error") + `],"nextCursor":null}`,
		},
	})
	before := f.requestCount()
	got := f.run("execution", "list", "--status", "success,error", "--all", "--limit", "1", "--workflow-id", "wf-1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if f.requestCount()-before != 4 {
		t.Fatalf("requests = %d, want two pages per status", f.requestCount()-before)
	}
	for i, want := range [][2]string{{"success", ""}, {"success", "s2"}, {"error", ""}, {"error", "e2"}} {
		query := f.request(before + i).URL.Query()
		if query.Get("status") != want[0] || query.Get("cursor") != want[1] || query.Get("workflowId") != "wf-1" || query.Get("limit") != "1" {
			t.Errorf("request %d query = %q, want status %s cursor %q with filters", i, f.request(before+i).URL.RawQuery, want[0], want[1])
		}
	}
	var page n8n.Page[n8n.Execution]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil {
		t.Fatalf("stdout = %q: %v", got.stdout, err)
	}
	if ids := executionIDs(page.Data); !slices.Equal(ids, []string{"10", "9", "6", "4"}) {
		t.Errorf("merged = %v, want [10 9 6 4]", ids)
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want no hint after a full walk", got.stderr)
	}
}

func TestExecutionListMultiStatusErrorStopsWithoutOutput(t *testing.T) {
	f := executionFixture(t, "")
	f.route = func(r *http.Request, _ int) (int, string) {
		if r.URL.Query().Get("status") == "error" {
			return http.StatusForbidden, `{"message":"forbidden"}`
		}
		return http.StatusOK, `{"data":[` + statusExecution("10", "success") + `],"nextCursor":null}`
	}
	got := f.run("execution", "list", "--status", "success,error,crashed")
	if got.code != ExitError || got.stdout != "" {
		t.Fatalf("result = %+v, want an error and no partial output", got)
	}
	if !strings.Contains(got.stderr, "execution:list") || !strings.Contains(got.stderr, "discover --resource executions") {
		t.Errorf("stderr = %q, want the scope diagnostic", got.stderr)
	}
	if status := f.lastRequest().URL.Query().Get("status"); status != "error" {
		t.Errorf("last status = %q, want the fan-out to stop at the failing status", status)
	}
}

func TestExecutionListMultiStatusScriptViews(t *testing.T) {
	pages := map[string]map[string]string{
		"success": {"": `{"data":[` + statusExecution("10", "success") + `,` + statusExecution("7", "success") + `],"nextCursor":"s2"}`},
		"error":   {"": `{"data":[` + statusExecution("9", "error") + `,` + statusExecution("7", "error") + `],"nextCursor":"e2"}`},
	}
	for name, args := range map[string][]string{
		"comma":    {"--status", "success,error"},
		"repeated": {"--status", "success", "--status", "error"},
	} {
		for flag, want := range map[string]string{
			"--quiet": "10\n9\n7\n",
			"--brief": "10\twf-1\tsuccess\n9\twf-1\terror\n7\twf-1\tsuccess\n",
		} {
			t.Run(name+flag, func(t *testing.T) {
				f := executionFixture(t, "")
				f.route = statusRoute(pages)
				before := f.requestCount()
				got := f.run(append([]string{"execution", "list", "--limit", "3", flag}, args...)...)
				if got.code != ExitSuccess || got.stdout != want {
					t.Fatalf("result = %+v, want stdout %q", got, want)
				}
				for i := before; i < f.requestCount(); i++ {
					if limit := f.request(i).URL.Query().Get("limit"); limit != "3" {
						t.Errorf("request %d limit = %q, want the explicit 3", i, limit)
					}
				}
				if !strings.Contains(got.stderr, "More executions match these statuses") {
					t.Errorf("stderr = %q, want the not-exhaustive hint", got.stderr)
				}
				if strings.Contains(got.stderr, "--cursor s2") || strings.Contains(got.stderr, "--cursor e2") || strings.Contains(got.stderr, "for the next page") {
					t.Errorf("stderr = %q, want no per-status or combined cursor", got.stderr)
				}
			})
		}
	}
}

func TestExecutionListMultiStatusScriptViewsDefaultLimitAndAll(t *testing.T) {
	f := executionFixture(t, "")
	f.route = statusRoute(map[string]map[string]string{
		"success": {
			"":   `{"data":[` + statusExecution("10", "success") + `],"nextCursor":"s2"}`,
			"s2": `{"data":[` + statusExecution("4", "success") + `],"nextCursor":null}`,
		},
		"error": {"": `{"data":[` + statusExecution("9", "error") + `],"nextCursor":null}`},
	})
	before := f.requestCount()
	got := f.run("execution", "list", "--status", "success,error", "--quiet")
	if got.code != ExitSuccess || got.stdout != "10\n9\n" {
		t.Fatalf("result = %+v, want the first page of each status merged", got)
	}
	for i := before; i < f.requestCount(); i++ {
		if limit := f.request(i).URL.Query().Get("limit"); limit != "30" {
			t.Errorf("request %d limit = %q, want the multi-status default 30", i, limit)
		}
	}
	got = f.run("execution", "list", "--status", "success,error", "--all", "--brief")
	if got.code != ExitSuccess || got.stdout != "10\twf-1\tsuccess\n9\twf-1\terror\n4\twf-1\tsuccess\n" || got.stderr != "" {
		t.Errorf("--all result = %+v, want every row and no hint", got)
	}
}

func TestExecutionListMultiStatusScriptViewErrorPrintsNoRows(t *testing.T) {
	f := executionFixture(t, "")
	f.route = func(r *http.Request, _ int) (int, string) {
		if r.URL.Query().Get("status") == "error" {
			return http.StatusForbidden, `{"message":"forbidden"}`
		}
		return http.StatusOK, `{"data":[` + statusExecution("10", "success") + `],"nextCursor":null}`
	}
	for _, flag := range []string{"--quiet", "--brief"} {
		got := f.run("execution", "list", "--status", "success,error", flag)
		if got.code != ExitError || got.stdout != "" || !strings.Contains(got.stderr, "execution:list") {
			t.Errorf("%s result = %+v, want the scope error and no partial rows", flag, got)
		}
	}
}

func TestExecutionListScriptViewsKeepPreTransportChecks(t *testing.T) {
	f := executionFixture(t, `{"data":[],"nextCursor":null}`)
	before := f.requestCount()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--status", "success,error", "--cursor", "c1", "--quiet"}, "--cursor needs a single --status"},
		{[]string{"--status", "bogus", "--brief"}, "bogus"},
		{[]string{"--all", "--include-data", "--quiet"}, "--all cannot be combined with --include-data"},
		{[]string{"--status", "success,error", "--quiet", "--output", "json"}, "cannot be combined with --output json"},
	} {
		got := f.run(append([]string{"execution", "list"}, tc.args...)...)
		if got.code != ExitError || !strings.Contains(got.stderr, tc.want) {
			t.Errorf("%v result = %+v, want %q", tc.args, got, tc.want)
		}
	}
	if f.requestCount() != before {
		t.Error("a rejected execution list reached the instance")
	}
}

// bulkStatusRoute serves each status's executions, given newest first, in
// pages of 250 chained by "<status>-<page>" cursors.
func bulkStatusRoute(ids map[string][]int) func(*http.Request, int) (int, string) {
	const size = 250
	return func(r *http.Request, _ int) (int, string) {
		status := r.URL.Query().Get("status")
		list, ok := ids[status]
		if !ok {
			return http.StatusBadRequest, `{"message":"unexpected status"}`
		}
		index := 0
		if cursor := r.URL.Query().Get("cursor"); cursor != "" {
			if _, err := fmt.Sscanf(cursor, status+"-%d", &index); err != nil {
				return http.StatusBadRequest, `{"message":"bad cursor"}`
			}
		}
		var body strings.Builder
		body.WriteString(`{"data":[`)
		for i := index * size; i < len(list) && i < (index+1)*size; i++ {
			if i > index*size {
				body.WriteByte(',')
			}
			body.WriteString(statusExecution(strconv.Itoa(list[i]), status))
		}
		next := "null"
		if (index+1)*size < len(list) {
			next = strconv.Quote(fmt.Sprintf("%s-%d", status, index+1))
		}
		fmt.Fprintf(&body, `],"nextCursor":%s}`, next)
		return http.StatusOK, body.String()
	}
}

// descendingIDs returns n IDs from first down, stepping by step.
func descendingIDs(first, n, step int) []int {
	ids := make([]int, n)
	for i := range ids {
		ids[i] = first - i*step
	}
	return ids
}

func TestExecutionListMultiStatusAllReportsCompleteness(t *testing.T) {
	tests := []struct {
		name      string
		ids       map[string][]int
		rows      int
		truncated bool
	}{
		{
			name: "one status stops at its cap while another exhausts",
			ids:  map[string][]int{"success": descendingIDs(100_000, 10_001, 2), "error": descendingIDs(99_999, 10, 2)},
			rows: 10_000, truncated: true,
		},
		{
			name: "exhausted statuses whose union exceeds the cap",
			ids:  map[string][]int{"success": descendingIDs(20_000, 6_000, 2), "error": descendingIDs(19_999, 6_000, 2)},
			rows: 10_000, truncated: true,
		},
		{
			name: "exactly the cap of unique rows",
			ids:  map[string][]int{"success": descendingIDs(20_000, 5_000, 2), "error": descendingIDs(19_999, 5_000, 2)},
			rows: 10_000,
		},
		{
			name: "duplicates alone are not truncation",
			ids:  map[string][]int{"success": descendingIDs(20_000, 6_000, 1), "error": descendingIDs(20_000, 6_000, 1)},
			rows: 6_000,
		},
		{
			name: "a truncated status stays incomplete after deduplication",
			ids:  map[string][]int{"success": descendingIDs(20_000, 10_001, 1), "error": descendingIDs(20_000, 10, 1)},
			rows: 10_000, truncated: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := executionFixture(t, "")
			f.route = bulkStatusRoute(tt.ids)
			got := f.run("execution", "list", "--status", "success,error", "--all", "--limit", "250", "--output", "json")
			if got.code != ExitSuccess {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			page, rows := decodeCollected(t, got.stdout)
			if rows != tt.rows {
				t.Errorf("rows = %d, want %d", rows, tt.rows)
			}
			if c := page.Collection; c == nil || c.Truncated != tt.truncated || c.Limit != n8n.DefaultCollectLimit {
				t.Errorf("collection = %+v, want truncated %t at the 10000 limit", page.Collection, tt.truncated)
			}
			if page.NextCursor == nil || *page.NextCursor != "" {
				t.Errorf("nextCursor = %v, want empty: a merged stream has no single cursor", page.NextCursor)
			}
			if tt.truncated {
				if !strings.Contains(got.stderr, "Warning: collection stopped") || !strings.Contains(got.stderr, "Repeating --all does not reach") {
					t.Errorf("stderr = %q, want the merged cap warning", got.stderr)
				}
			} else if got.stderr != "" {
				t.Errorf("stderr = %q, want no warning", got.stderr)
			}
		})
	}
}

func TestExecutionListMultiStatusPageReportsCollection(t *testing.T) {
	f := executionFixture(t, "")
	f.route = bulkStatusRoute(map[string][]int{"success": {10, 7}, "error": {9}})
	got := f.run("execution", "list", "--status", "success,error", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	page, rows := decodeCollected(t, got.stdout)
	if c := page.Collection; rows != 3 || c == nil || c.Truncated || c.Limit != executionMultiStatusPageSize {
		t.Errorf("rows %d collection %+v, want 3 rows, complete at the merged limit 30", rows, page.Collection)
	}

	got = f.run("execution", "list", "--status", "success,error", "--limit", "2", "--output", "json")
	page, _ = decodeCollected(t, got.stdout)
	if c := page.Collection; c == nil || !c.Truncated || c.Limit != 2 {
		t.Errorf("collection = %+v, want truncated at the explicit limit 2", page.Collection)
	}
	if !strings.Contains(got.stderr, "More executions match these statuses") || strings.Contains(got.stderr, "Warning") {
		t.Errorf("stderr = %q, want the page-mode hint only", got.stderr)
	}
}
