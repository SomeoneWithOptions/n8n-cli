package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
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
