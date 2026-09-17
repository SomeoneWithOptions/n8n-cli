package n8n

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

const evaluationRunSummaryResponse = `{
  "id":"run-1",
  "status":"completed",
  "runAt":"2026-09-18T10:00:00Z",
  "completedAt":"2026-09-18T10:01:00Z",
  "metrics":{"accuracy":0.95},
  "errorCode":null,
  "errorDetails":null,
  "finalResult":"success",
  "testCaseCount":2,
  "createdAt":"2026-09-18T09:59:00Z",
  "updatedAt":"2026-09-18T10:01:00Z"
}`

const evaluationCaseResponse = `{
  "id":"case-1",
  "status":"success",
  "runAt":"2026-09-18T10:00:00Z",
  "completedAt":"2026-09-18T10:00:10Z",
  "metrics":{"quality":1},
  "errorCode":null,
  "errorDetails":null,
  "inputs":{"question":"hello"},
  "outputs":{"answer":"world"},
  "executionId":"1000"
}`

func TestListEvaluationRunsQueryAndDecoding(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK,
		`{"data":[`+evaluationRunSummaryResponse+`],"nextCursor":"next/one?x=1"}`))
	page, err := server.client(t).ListEvaluationRuns(context.Background(), "wf/one?x=1", ListEvaluationRunsOptions{
		ListOptions: ListOptions{Limit: 50, Cursor: "prev/one?x=1"},
		Status:      "completed",
	})
	if err != nil {
		t.Fatalf("ListEvaluationRuns: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+"/workflows/wf%2Fone%3Fx=1/test-runs" || body != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.EscapedPath(), body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if req.URL.Query().Get("limit") != "50" || req.URL.Query().Get("cursor") != "prev/one?x=1" || req.URL.Query().Get("status") != "completed" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}
	if len(page.Data) != 1 || page.NextCursor != "next/one?x=1" {
		t.Fatalf("page = %+v", page)
	}
	run := page.Data[0]
	if run.ID != "run-1" || run.Status != EvaluationRunCompleted || run.FinalResult == nil || *run.FinalResult != "success" || run.TestCaseCount != 2 {
		t.Errorf("run = %+v", run)
	}
	if len(run.Metrics) == 0 || string(run.Metrics) != `{"accuracy":0.95}` {
		t.Errorf("metrics = %s", run.Metrics)
	}
}

func TestEvaluationRequests(t *testing.T) {
	workflowID, runID := "wf/one?x=1", "run/two?x=2"
	tests := []struct {
		name   string
		call   func(*Client) error
		method string
		path   string
		query  string
		result string
		status int
	}{
		{name: "create", call: func(c *Client) error {
			run, err := c.CreateEvaluationRun(context.Background(), workflowID)
			if err == nil && (run.ID != "run-1" || run.Status != EvaluationRunNew) {
				t.Errorf("run = %+v", run)
			}
			return err
		}, method: http.MethodPost, path: "/workflows/wf%2Fone%3Fx=1/test-runs", result: `{"id":"run-1","status":"new","createdAt":"2026-09-18T09:59:00Z"}`, status: http.StatusCreated},
		{name: "get", call: func(c *Client) error {
			run, err := c.GetEvaluationRun(context.Background(), workflowID, runID)
			if err == nil && run.ID != "run-1" {
				t.Errorf("run = %+v", run)
			}
			return err
		}, method: http.MethodGet, path: "/workflows/wf%2Fone%3Fx=1/test-runs/run%2Ftwo%3Fx=2", result: evaluationRunSummaryResponse, status: http.StatusOK},
		{name: "cancel", call: func(c *Client) error {
			result, err := c.CancelEvaluationRun(context.Background(), workflowID, runID)
			if err == nil && (result.ID != "run-1" || result.Status != EvaluationRunCancelled) {
				t.Errorf("result = %+v", result)
			}
			return err
		}, method: http.MethodPost, path: "/workflows/wf%2Fone%3Fx=1/test-runs/run%2Ftwo%3Fx=2/cancel", result: `{"id":"run-1","status":"cancelled"}`, status: http.StatusAccepted},
		{name: "cases", call: func(c *Client) error {
			page, err := c.ListEvaluationCases(context.Background(), workflowID, runID, ListEvaluationCasesOptions{ListOptions: ListOptions{Limit: 25, Cursor: "case/cursor?x=1"}})
			if err == nil && (len(page.Data) != 1 || page.Data[0].ID != "case-1" || page.Data[0].ExecutionID == nil || *page.Data[0].ExecutionID != "1000") {
				t.Errorf("page = %+v", page)
			}
			return err
		}, method: http.MethodGet, path: "/workflows/wf%2Fone%3Fx=1/test-runs/run%2Ftwo%3Fx=2/test-cases", query: "cursor=case%2Fcursor%3Fx%3D1&limit=25", result: `{"data":[` + evaluationCaseResponse + `],"nextCursor":"next"}`, status: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tt.status, tt.result))
			if err := tt.call(server.client(t)); err != nil {
				t.Fatalf("call: %v", err)
			}
			req, body := server.last(t)
			if req.Method != tt.method || req.URL.EscapedPath() != BasePath+tt.path {
				t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.EscapedPath(), tt.method, BasePath+tt.path)
			}
			if req.URL.RawQuery != tt.query {
				t.Errorf("query = %q, want %q", req.URL.RawQuery, tt.query)
			}
			if body != "" {
				t.Errorf("body = %q, want empty", body)
			}
		})
	}
}

func TestEvaluationValidationBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))
	client := server.client(t)
	calls := map[string]func() error{
		"empty workflow": func() error {
			_, err := client.ListEvaluationRuns(context.Background(), " ", ListEvaluationRunsOptions{})
			return err
		},
		"padded workflow": func() error {
			_, err := client.CreateEvaluationRun(context.Background(), " wf-1 ")
			return err
		},
		"empty run": func() error {
			_, err := client.GetEvaluationRun(context.Background(), "wf-1", "")
			return err
		},
		"padded run": func() error {
			_, err := client.CancelEvaluationRun(context.Background(), "wf-1", " run-1 ")
			return err
		},
		"bad status": func() error {
			_, err := client.ListEvaluationRuns(context.Background(), "wf-1", ListEvaluationRunsOptions{Status: "success"})
			return err
		},
		"run limit": func() error {
			_, err := client.ListEvaluationRuns(context.Background(), "wf-1", ListEvaluationRunsOptions{ListOptions: ListOptions{Limit: 251}})
			return err
		},
		"case limit": func() error {
			_, err := client.ListEvaluationCases(context.Background(), "wf-1", "run-1", ListEvaluationCasesOptions{ListOptions: ListOptions{Limit: 251}})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("%d invalid requests reached instance", len(server.requests))
	}
}

func TestEvaluationErrorsPreserveAPIError(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusConflict, `{"message":"run is already completed"}`))
	_, err := server.client(t).CancelEvaluationRun(context.Background(), "wf-1", "run-1")
	if err == nil || !IsConflict(err) || !strings.Contains(err.Error(), "already completed") {
		t.Fatalf("CancelEvaluationRun error = %v", err)
	}
}
