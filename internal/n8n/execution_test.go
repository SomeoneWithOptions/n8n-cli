package n8n

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

const executionResponse = `{
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
  "tracingContext":{"traceparent":"00-abc-def-01"},
  "deduplicationKey":null,
  "jsonSizeBytes":2048,
  "binaryDataSizeBytes":10,
  "workflowVersionId":"ver-1",
  "usedPrivateCredentials":false,
  "data":{"resultData":{"runData":{}},"redactionInfo":null},
  "workflowData":{"id":"wf-1","name":"Invoice sync"},
  "customData":{"region":"eu"},
  "dataTooLargeToDisplay":false
}`

const deletedExecutionResponse = `{
  "id":1000,
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
  "binaryDataSizeBytes":10,
  "workflowVersionId":"ver-1",
  "usedPrivateCredentials":false
}`

func TestListExecutionsQueryAndDecoding(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK,
		`{"data":[`+executionResponse+`],"nextCursor":"next/one?x=1"}`))
	redact := false
	page, err := server.client(t).ListExecutions(context.Background(), ListExecutionsOptions{
		ListOptions: ListOptions{Limit: 50, Cursor: "prev/one?x=1"},
		ExecutionDataOptions: ExecutionDataOptions{
			IncludeData: true, IgnoreDataSizeLimit: true, RedactExecutionData: &redact,
		},
		Status: "success", WorkflowID: "wf-1", ProjectID: "pr-1",
		StartedAfter: "2026-09-17T00:00:00Z", StartedBefore: "2026-09-18T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+ExecutionsPath || body != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.Path, body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	for field, want := range map[string]string{
		"limit": "50", "cursor": "prev/one?x=1", "includeData": "true",
		"ignoreDataSizeLimit": "true", "redactExecutionData": "false", "status": "success",
		"workflowId": "wf-1", "projectId": "pr-1", "startedAfter": "2026-09-17T00:00:00Z",
		"startedBefore": "2026-09-18T00:00:00Z",
	} {
		if got := req.URL.Query().Get(field); got != want {
			t.Errorf("query %s = %q, want %q", field, got, want)
		}
	}
	if len(page.Data) != 1 || page.NextCursor != "next/one?x=1" {
		t.Fatalf("page = %+v", page)
	}
	execution := page.Data[0]
	if execution.ID != "1000" || execution.WorkflowID != "wf-1" || execution.Status != "success" {
		t.Errorf("execution = %+v", execution)
	}
	if execution.JSONSizeBytes != 2048 || len(execution.Data) == 0 || len(execution.WorkflowData) == 0 {
		t.Errorf("detailed execution fields = %+v", execution)
	}
}

func TestExecutionRequests(t *testing.T) {
	id := "1000/one?x=1"
	tests := []struct {
		name   string
		call   func(*Client) error
		method string
		path   string
		query  string
		body   string
		result string
	}{
		{name: "get metadata", call: func(c *Client) error {
			execution, err := c.GetExecution(context.Background(), id, ExecutionDataOptions{})
			if err == nil && execution.ID != "1000" {
				t.Errorf("execution = %+v", execution)
			}
			return err
		}, method: http.MethodGet, path: "/executions/1000%2Fone%3Fx=1", result: executionResponse},
		{name: "get detailed and redacted", call: func(c *Client) error {
			redact := true
			_, err := c.GetExecution(context.Background(), id, ExecutionDataOptions{IncludeData: true, RedactExecutionData: &redact})
			return err
		}, method: http.MethodGet, path: "/executions/1000%2Fone%3Fx=1", query: "includeData=true&redactExecutionData=true", result: executionResponse},
		{name: "delete", call: func(c *Client) error {
			deleted, err := c.DeleteExecution(context.Background(), id)
			if err == nil && deleted.ID.String() != "1000" {
				t.Errorf("deleted = %+v", deleted)
			}
			return err
		}, method: http.MethodDelete, path: "/executions/1000%2Fone%3Fx=1", result: deletedExecutionResponse},
		{name: "tag list", call: func(c *Client) error {
			tags, err := c.GetExecutionTags(context.Background(), id)
			if err == nil && (len(tags) != 1 || tags[0].ID != "tag-1") {
				t.Errorf("tags = %+v", tags)
			}
			return err
		}, method: http.MethodGet, path: "/executions/1000%2Fone%3Fx=1/tags", result: `[{"id":"tag-1","name":"Production"}]`},
		{name: "tag set", call: func(c *Client) error {
			_, err := c.SetExecutionTags(context.Background(), id, []string{"tag-1", "tag-2"})
			return err
		}, method: http.MethodPut, path: "/executions/1000%2Fone%3Fx=1/tags", body: `[{"id":"tag-1"},{"id":"tag-2"}]`, result: `[{"id":"tag-1","name":"Production"}]`},
		{name: "tag clear", call: func(c *Client) error {
			_, err := c.SetExecutionTags(context.Background(), id, nil)
			return err
		}, method: http.MethodPut, path: "/executions/1000%2Fone%3Fx=1/tags", body: `[]`, result: `[]`},
		{name: "stop many", call: func(c *Client) error {
			result, err := c.StopManyExecutions(context.Background(), StopManyExecutionsRequest{
				Status: []string{"queued", "running"}, WorkflowID: "wf-1",
				StartedAfter: "2026-09-17T00:00:00Z", StartedBefore: "2026-09-18T00:00:00Z",
			})
			if err == nil && result.Stopped != 2 {
				t.Errorf("result = %+v", result)
			}
			return err
		}, method: http.MethodPost, path: "/executions/stop", body: `{"status":["queued","running"],"workflowId":"wf-1","startedAfter":"2026-09-17T00:00:00Z","startedBefore":"2026-09-18T00:00:00Z"}`, result: `{"stopped":2}`},
		{name: "stop one", call: func(c *Client) error {
			result, err := c.StopExecution(context.Background(), id)
			if err == nil && result.Status != "canceled" {
				t.Errorf("result = %+v", result)
			}
			return err
		}, method: http.MethodPost, path: "/executions/1000%2Fone%3Fx=1/stop", result: `{"mode":"manual","startedAt":"2026-09-17T10:00:01Z","stoppedAt":"2026-09-17T10:00:02Z","finished":false,"status":"canceled"}`},
		{name: "retry stored workflow", call: func(c *Client) error {
			_, err := c.RetryExecution(context.Background(), id, RetryExecutionOptions{})
			return err
		}, method: http.MethodPost, path: "/executions/1000%2Fone%3Fx=1/retry", body: `{}`, result: executionResponse},
		{name: "retry latest workflow", call: func(c *Client) error {
			_, err := c.RetryExecution(context.Background(), id, RetryExecutionOptions{LoadWorkflow: true})
			return err
		}, method: http.MethodPost, path: "/executions/1000%2Fone%3Fx=1/retry", body: `{"loadWorkflow":true}`, result: executionResponse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, tt.result))
			if err := tt.call(server.client(t)); err != nil {
				t.Fatalf("call: %v", err)
			}
			req, body := server.last(t)
			if req.Method != tt.method {
				t.Errorf("method = %q, want %q", req.Method, tt.method)
			}
			if want := BasePath + tt.path; req.URL.EscapedPath() != want {
				t.Errorf("path = %q, want %q", req.URL.EscapedPath(), want)
			}
			if req.URL.RawQuery != tt.query {
				t.Errorf("query = %q, want %q", req.URL.RawQuery, tt.query)
			}
			if tt.body == "" {
				if body != "" {
					t.Errorf("body = %q, want empty", body)
				}
			} else {
				assertJSONEqual(t, tt.body, body)
			}
		})
	}
}

func TestExecutionValidationBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, executionResponse))
	client := server.client(t)
	calls := map[string]func() error{
		"list limit": func() error {
			_, err := client.ListExecutions(context.Background(), ListExecutionsOptions{ListOptions: ListOptions{Limit: 251}})
			return err
		},
		"list status": func() error {
			_, err := client.ListExecutions(context.Background(), ListExecutionsOptions{Status: "queued"})
			return err
		},
		"list time": func() error {
			_, err := client.ListExecutions(context.Background(), ListExecutionsOptions{StartedAfter: "yesterday"})
			return err
		},
		"list reversed range": func() error {
			_, err := client.ListExecutions(context.Background(), ListExecutionsOptions{StartedAfter: "2026-09-18T00:00:00Z", StartedBefore: "2026-09-17T00:00:00Z"})
			return err
		},
		"empty ID": func() error {
			_, err := client.GetExecution(context.Background(), " ", ExecutionDataOptions{})
			return err
		},
		"padded ID": func() error {
			_, err := client.StopExecution(context.Background(), " 1000 ")
			return err
		},
		"empty stop statuses": func() error {
			_, err := client.StopManyExecutions(context.Background(), StopManyExecutionsRequest{})
			return err
		},
		"bad stop status": func() error {
			_, err := client.StopManyExecutions(context.Background(), StopManyExecutionsRequest{Status: []string{"success"}})
			return err
		},
		"duplicate stop status": func() error {
			_, err := client.StopManyExecutions(context.Background(), StopManyExecutionsRequest{Status: []string{"running", "running"}})
			return err
		},
		"empty tag ID": func() error {
			_, err := client.SetExecutionTags(context.Background(), "1000", []string{""})
			return err
		},
		"duplicate tag ID": func() error {
			_, err := client.SetExecutionTags(context.Background(), "1000", []string{"tag-1", "tag-1"})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("%d invalid requests reached the instance", len(server.requests))
	}
}

func TestExecutionErrorsPreserveAPIError(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusConflict, `{"message":"already finished"}`))
	_, err := server.client(t).StopExecution(context.Background(), "1000")
	if err == nil || !IsConflict(err) || !strings.Contains(err.Error(), "already finished") {
		t.Fatalf("StopExecution error = %v", err)
	}
}

func TestValidateExecutionStatuses(t *testing.T) {
	for name, tt := range map[string]struct {
		statuses []string
		want     string
	}{
		"none":      {statuses: nil},
		"single":    {statuses: []string{"error"}},
		"many":      {statuses: []string{"success", "error", "crashed"}},
		"empty":     {statuses: []string{"success", ""}, want: "status 2 is empty: use canceled, crashed"},
		"unknown":   {statuses: []string{"success", "failed"}, want: `unknown execution status "failed": use canceled`},
		"queued":    {statuses: []string{"queued"}, want: `unknown execution status "queued"`},
		"duplicate": {statuses: []string{"error", "success", "error"}, want: `status "error" is listed twice`},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateExecutionStatuses(tt.statuses)
			if tt.want == "" {
				if err != nil {
					t.Errorf("err = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}
