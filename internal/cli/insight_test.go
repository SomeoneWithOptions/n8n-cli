package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliInsightsSummary = `{
  "total":{"value":120,"deviation":20.5,"unit":"count"},
  "failed":{"value":6,"deviation":-2,"unit":"count"},
  "failureRate":{"value":0.05,"deviation":null,"unit":"ratio"},
  "timeSaved":{"value":345.5,"deviation":12.25,"unit":"minute"},
  "averageRunTime":{"value":842.75,"deviation":-15.5,"unit":"millisecond"}
}`

func insightFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = cliInsightsSummary
	return f
}

func TestInsightSummaryTextAndFilters(t *testing.T) {
	f := insightFixture(t)
	got := f.run("insight", "summary",
		"--start-date", "2026-09-01T00:00:00Z",
		"--end-date", "2026-09-17T23:59:59+02:00",
		"--project-id", "project/one?x=1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{
		"Insights period start:", "2026-09-01T00:00:00Z", "Insights period end:",
		"project/one?x=1", "METRIC", "Total executions", "120", "count", "20.5",
		"Failed executions", "Failure rate", "0.05", "ratio", "Time saved", "minute",
		"Average run time", "millisecond",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+"/insights/summary" || f.lastBody() != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), f.lastBody())
	}
	query := req.URL.Query()
	if query.Get("startDate") != "2026-09-01T00:00:00Z" || query.Get("endDate") != "2026-09-17T23:59:59+02:00" || query.Get("projectId") != "project/one?x=1" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}
}

func TestInsightSummaryJSONIsStableAndDefaultsStayServerSide(t *testing.T) {
	f := insightFixture(t)
	got := f.run("insight", "summary", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	const want = `{
  "total": {
    "value": 120,
    "deviation": 20.5,
    "unit": "count"
  },
  "failed": {
    "value": 6,
    "deviation": -2,
    "unit": "count"
  },
  "failureRate": {
    "value": 0.05,
    "deviation": null,
    "unit": "ratio"
  },
  "timeSaved": {
    "value": 345.5,
    "deviation": 12.25,
    "unit": "minute"
  },
  "averageRunTime": {
    "value": 842.75,
    "deviation": -15.5,
    "unit": "millisecond"
  }
}
`
	if got.stdout != want {
		t.Errorf("stdout =\n%s\nwant stable JSON =\n%s", got.stdout, want)
	}
	if query := f.lastRequest().URL.RawQuery; query != "" {
		t.Errorf("query = %q, want empty so server date and project defaults apply", query)
	}
	var summary n8n.InsightsSummary
	if err := json.Unmarshal([]byte(got.stdout), &summary); err != nil || summary.FailureRate.Deviation != nil {
		t.Errorf("stdout did not preserve null deviation: %v (error: %v)", summary.FailureRate, err)
	}
}

func TestInsightSummaryValidationBeforeTransport(t *testing.T) {
	tests := map[string][]string{
		"start date":     {"insight", "summary", "--start-date", "yesterday"},
		"end date":       {"insight", "summary", "--end-date", "tomorrow"},
		"reversed range": {"insight", "summary", "--start-date", "2026-09-18T00:00:00Z", "--end-date", "2026-09-17T00:00:00Z"},
		"project ID":     {"insight", "summary", "--project-id", " project-1 "},
		"output format":  {"insight", "summary", "--output", "yaml"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			f := insightFixture(t)
			before := f.requestCount()
			got := f.run(args...)
			if got.code != ExitError || got.stderr == "" {
				t.Errorf("result = %+v", got)
			}
			if f.requestCount() != before {
				t.Error("invalid summary request reached instance")
			}
		})
	}
}

func TestInsightSummaryAPIErrorsExplainFix(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   []string
	}{
		{name: "bad filters", status: http.StatusBadRequest, want: []string{"rejected the insight filters", "RFC3339 date range", "project ID"}},
		{name: "forbidden", status: http.StatusForbidden, want: []string{"insights:read", "selected project", "discover --resource insights"}},
		{name: "unauthorized", status: http.StatusUnauthorized, want: []string{"rejected the credential", "auth login"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := insightFixture(t)
			f.status = tt.status
			f.body = `{"message":"denied"}`
			got := f.run("insight", "summary")
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
