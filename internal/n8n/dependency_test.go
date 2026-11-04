package n8n

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

const dependencyResponse = `{
  "data":[
    {"type":"credential","id":"cred1","name":"GitHub","projectId":"project1"},
    {"type":"parentWorkflow","id":"wf2","name":"Caller"}
  ],
  "nextCursor":"next one",
  "inaccessibleCount":2
}`

// TestDependencyLookups covers the three dependency endpoints: method, escaped
// path, pagination query, and the shared response shape.
func TestDependencyLookups(t *testing.T) {
	tests := []struct {
		name    string
		call    func(*Client, context.Context, string, ListOptions) (DependencyPage, error)
		id      string
		escaped string
	}{
		{name: "workflow", call: (*Client).WorkflowDependencies, id: "wf/one?x=1", escaped: "/workflows/wf%2Fone%3Fx=1/dependencies"},
		{name: "credential", call: (*Client).CredentialDependencies, id: "cred/one?x=1", escaped: "/credentials/cred%2Fone%3Fx=1/dependencies"},
		{name: "data table", call: (*Client).DataTableDependencies, id: "table/one?x=1", escaped: "/data-tables/table%2Fone%3Fx=1/dependencies"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, dependencyResponse))
			page, err := tt.call(server.client(t), context.Background(), tt.id, ListOptions{Limit: 25, Cursor: "start one"})
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			req, body := server.last(t)
			if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+tt.escaped {
				t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.EscapedPath(), BasePath+tt.escaped)
			}
			if req.URL.Query().Get("limit") != "25" || req.URL.Query().Get("cursor") != "start one" {
				t.Errorf("query = %q, want pagination", req.URL.RawQuery)
			}
			if body != "" {
				t.Errorf("body = %q, want empty", body)
			}
			if len(page.Data) != 2 || page.NextCursor != "next one" || page.InaccessibleCount != 2 || !page.HasMore() {
				t.Fatalf("page = %+v", page)
			}
			first, second := page.Data[0], page.Data[1]
			if first != (Dependency{Type: DependencyCredential, ID: "cred1", Name: "GitHub", ProjectID: "project1"}) {
				t.Errorf("first = %+v", first)
			}
			if second != (Dependency{Type: DependencyParentWorkflow, ID: "wf2", Name: "Caller"}) {
				t.Errorf("second = %+v", second)
			}
			generic := page.Page()
			if len(generic.Data) != 2 || generic.NextCursor != "next one" {
				t.Errorf("Page() = %+v", generic)
			}
		})
	}
}

// TestDependencyLastPage decodes a null cursor and an empty list into the
// documented zero values, so callers never see a nil slice.
func TestDependencyLastPage(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null,"inaccessibleCount":0}`))
	page, err := server.client(t).WorkflowDependencies(context.Background(), "wf1", ListOptions{})
	if err != nil {
		t.Fatalf("WorkflowDependencies: %v", err)
	}
	if page.Data == nil || len(page.Data) != 0 || page.HasMore() || page.InaccessibleCount != 0 {
		t.Errorf("page = %+v", page)
	}
	req, _ := server.last(t)
	if req.URL.RawQuery != "" {
		t.Errorf("query = %q, want none for default options", req.URL.RawQuery)
	}
}

func TestDependencyValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client) error
		want string
	}{
		{name: "workflow ID", want: "workflow ID is required", call: func(c *Client) error {
			_, err := c.WorkflowDependencies(context.Background(), " ", ListOptions{})
			return err
		}},
		{name: "credential ID", want: "credential ID must not start or end with whitespace", call: func(c *Client) error {
			_, err := c.CredentialDependencies(context.Background(), " cred", ListOptions{})
			return err
		}},
		{name: "data table ID", want: "data table ID is required", call: func(c *Client) error {
			_, err := c.DataTableDependencies(context.Background(), "", ListOptions{})
			return err
		}},
		{name: "negative limit", want: "limit must not be negative", call: func(c *Client) error {
			_, err := c.WorkflowDependencies(context.Background(), "wf1", ListOptions{Limit: -1})
			return err
		}},
		{name: "limit above maximum", want: "limit must not exceed 250", call: func(c *Client) error {
			_, err := c.CredentialDependencies(context.Background(), "cred1", ListOptions{Limit: 251})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, dependencyResponse))
			err := tt.call(server.client(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if len(server.requests) != 0 {
				t.Errorf("%d requests reached server, want none", len(server.requests))
			}
		})
	}
}

func TestDependencyErrorsKeepStatus(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		server := newCommunityPackageServer(t, packageJSONHandler(status, `{"message":"denied"}`))
		_, err := server.client(t).DataTableDependencies(context.Background(), "table1", ListOptions{})
		if StatusCodeOf(err) != status {
			t.Errorf("status = %d, want %d (err %v)", StatusCodeOf(err), status, err)
		}
	}
}
