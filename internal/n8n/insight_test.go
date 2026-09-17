package n8n

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

const insightsSummaryResponse = `{
  "total":{"value":120,"deviation":20.5,"unit":"count"},
  "failed":{"value":6,"deviation":-2,"unit":"count"},
  "failureRate":{"value":0.05,"deviation":null,"unit":"ratio"},
  "timeSaved":{"value":345.5,"deviation":12.25,"unit":"minute"},
  "averageRunTime":{"value":842.75,"deviation":-15.5,"unit":"millisecond"}
}`

func TestGetInsightsSummaryQueryAndDecoding(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, insightsSummaryResponse))
	summary, err := server.client(t).GetInsightsSummary(context.Background(), GetInsightsSummaryOptions{
		StartDate: "2026-09-01T00:00:00Z",
		EndDate:   "2026-09-17T23:59:59+02:00",
		ProjectID: "project/one?x=1",
	})
	if err != nil {
		t.Fatalf("GetInsightsSummary: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+"/insights/summary" || body != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	query := req.URL.Query()
	if query.Get("startDate") != "2026-09-01T00:00:00Z" || query.Get("endDate") != "2026-09-17T23:59:59+02:00" || query.Get("projectId") != "project/one?x=1" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}
	if summary.Total.Value != 120 || summary.Total.Unit != InsightUnitCount || summary.Total.Deviation == nil || *summary.Total.Deviation != 20.5 {
		t.Errorf("total = %+v", summary.Total)
	}
	if summary.FailureRate.Value != 0.05 || summary.FailureRate.Unit != InsightUnitRatio || summary.FailureRate.Deviation != nil {
		t.Errorf("failureRate = %+v", summary.FailureRate)
	}
	if summary.TimeSaved.Unit != InsightUnitMinute || summary.AverageRunTime.Unit != InsightUnitMillisecond {
		t.Errorf("units = %q, %q", summary.TimeSaved.Unit, summary.AverageRunTime.Unit)
	}
}

func TestGetInsightsSummaryOmitsEmptyFilters(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, insightsSummaryResponse))
	if _, err := server.client(t).GetInsightsSummary(context.Background(), GetInsightsSummaryOptions{}); err != nil {
		t.Fatalf("GetInsightsSummary: %v", err)
	}
	req, _ := server.last(t)
	if req.URL.RawQuery != "" {
		t.Errorf("query = %q, want empty so server defaults apply", req.URL.RawQuery)
	}
}

func TestInsightsSummaryValidationBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, insightsSummaryResponse))
	client := server.client(t)
	tests := map[string]GetInsightsSummaryOptions{
		"bad start":      {StartDate: "yesterday"},
		"bad end":        {EndDate: "tomorrow"},
		"reversed range": {StartDate: "2026-09-18T00:00:00Z", EndDate: "2026-09-17T23:59:59Z"},
		"padded project": {ProjectID: " project-1 "},
	}
	for name, opts := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := client.GetInsightsSummary(context.Background(), opts); err == nil {
				t.Error("want validation error")
			}
		})
	}
	if len(server.requests) != 0 {
		t.Errorf("%d invalid requests reached instance", len(server.requests))
	}
}

func TestInsightsSummaryErrorsPreserveAPIError(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusForbidden, `{"message":"missing insights:read"}`))
	_, err := server.client(t).GetInsightsSummary(context.Background(), GetInsightsSummaryOptions{})
	if err == nil || !IsForbidden(err) || !strings.Contains(err.Error(), "insights:read") {
		t.Fatalf("GetInsightsSummary error = %v", err)
	}
}
