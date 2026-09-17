package n8n

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const variableResponse = `{
  "id":"var/one?x=1",
  "key":"API_HOST",
  "value":"https://api.example.com",
  "type":"string",
  "project":{"id":"project-one","name":"Production","type":"team"},
  "extraFieldAddedLater":"ignored"
}`

var wantVariable = Variable{
	ID:    "var/one?x=1",
	Key:   "API_HOST",
	Value: "https://api.example.com",
	Type:  "string",
	Project: &VariableProject{
		ID: "project-one", Name: "Production", Type: "team",
	},
}

func TestListVariables(t *testing.T) {
	body := `{"data":[` + variableResponse + `],"nextCursor":"next page"}`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))

	page, err := server.client(t).ListVariables(context.Background(), ListVariablesOptions{
		ListOptions: ListOptions{Limit: 25, Cursor: "start here"},
		ProjectID:   "project/one?x=1",
		State:       VariableStateEmpty,
	})
	if err != nil {
		t.Fatalf("ListVariables: %v", err)
	}
	req, requestBody := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+VariablesPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+VariablesPath)
	}
	query := req.URL.Query()
	if query.Get("limit") != "25" || query.Get("cursor") != "start here" ||
		query.Get("projectId") != "project/one?x=1" || query.Get("state") != "empty" {
		t.Errorf("query = %q, want pagination, projectId and state", req.URL.RawQuery)
	}
	if requestBody != "" {
		t.Errorf("body = %q, want empty", requestBody)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if diff := cmp.Diff([]Variable{wantVariable}, page.Data); diff != "" {
		t.Errorf("page data mismatch (-want +got):\n%s", diff)
	}
	if page.NextCursor != "next page" || !page.HasMore() {
		t.Errorf("nextCursor = %q, HasMore = %v", page.NextCursor, page.HasMore())
	}
}

func TestListVariablesCollectsEveryPageWithFilters(t *testing.T) {
	pages := []string{
		`{"data":[{"id":"1","key":"FIRST","value":""}],"nextCursor":"page two"}`,
		`{"data":[{"id":"2","key":"SECOND","value":""}],"nextCursor":null}`,
	}
	var n int
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("projectId") != "project-one" || r.URL.Query().Get("state") != "empty" {
			t.Errorf("request %d lost filters: %q", n+1, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", contentTypeJSON)
		_, _ = w.Write([]byte(pages[n]))
		n++
	})
	client := server.client(t)
	base := ListVariablesOptions{ProjectID: "project-one", State: VariableStateEmpty}
	fetch := func(ctx context.Context, pagination ListOptions) (Page[Variable], error) {
		opts := base
		opts.ListOptions = pagination
		return client.ListVariables(ctx, opts)
	}

	variables, err := Collect(context.Background(), fetch, ListOptions{}, 0)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if diff := cmp.Diff([]Variable{{ID: "1", Key: "FIRST", Value: ""}, {ID: "2", Key: "SECOND", Value: ""}}, variables); diff != "" {
		t.Errorf("variables mismatch (-want +got):\n%s", diff)
	}
	if len(server.requests) != 2 || server.requests[1].URL.Query().Get("cursor") != "page two" {
		t.Errorf("requests = %d, second query = %q", len(server.requests), server.requests[1].URL.RawQuery)
	}
}

func TestCreateUpdateAndDeleteVariable(t *testing.T) {
	tests := []struct {
		name    string
		call    func(*Client) error
		status  int
		method  string
		escaped string
		body    string
	}{
		{name: "create global with empty value", call: func(c *Client) error {
			return c.CreateVariable(context.Background(), VariableRequest{Key: "OPTIONAL", Value: ""})
		}, status: http.StatusCreated, method: http.MethodPost, escaped: VariablesPath, body: `{"key":"OPTIONAL","value":""}`},
		{name: "create in project", call: func(c *Client) error {
			return c.CreateVariable(context.Background(), VariableRequest{Key: "API_HOST", Value: "secret-value", ProjectID: "project-one"})
		}, status: http.StatusCreated, method: http.MethodPost, escaped: VariablesPath, body: `{"key":"API_HOST","value":"secret-value","projectId":"project-one"}`},
		{name: "update", call: func(c *Client) error {
			return c.UpdateVariable(context.Background(), "var/one?x=1", VariableRequest{Key: "API_HOST", Value: "new-value", ProjectID: "project-one"})
		}, status: http.StatusNoContent, method: http.MethodPut, escaped: "/variables/var%2Fone%3Fx=1", body: `{"key":"API_HOST","value":"new-value","projectId":"project-one"}`},
		{name: "delete", call: func(c *Client) error {
			return c.DeleteVariable(context.Background(), "var/one?x=1")
		}, status: http.StatusNoContent, method: http.MethodDelete, escaped: "/variables/var%2Fone%3Fx=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tt.status) })
			if err := tt.call(server.client(t)); err != nil {
				t.Fatalf("call: %v", err)
			}
			req, body := server.last(t)
			if req.Method != tt.method || req.URL.EscapedPath() != BasePath+tt.escaped {
				t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.EscapedPath(), tt.method, BasePath+tt.escaped)
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

func TestVariableWriteErrorsRedactSubmittedValue(t *testing.T) {
	const value = "do-not-print-this-value"
	for _, update := range []bool{false, true} {
		name := map[bool]string{false: "create", true: "update"}[update]
		t.Run(name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusConflict, `{"message":"duplicate `+value+`","code":"`+value+`","hint":"`+value+`"}`))
			client := server.client(t)
			var err error
			if update {
				err = client.UpdateVariable(context.Background(), "id", VariableRequest{Key: "API_HOST", Value: value})
			} else {
				err = client.CreateVariable(context.Background(), VariableRequest{Key: "API_HOST", Value: value})
			}
			if !IsConflict(err) {
				t.Fatalf("error = %v, want 409", err)
			}
			if got := err.Error(); strings.Contains(got, value) || !strings.Contains(got, "details redacted") {
				t.Errorf("error = %q, want redaction without submitted value", got)
			}
		})
	}
}

func TestVariableValidationBeforeRequest(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{}`))
	client := server.client(t)
	calls := map[string]func() error{
		"negative limit": func() error {
			_, err := client.ListVariables(context.Background(), ListVariablesOptions{ListOptions: ListOptions{Limit: -1}})
			return err
		},
		"invalid state": func() error {
			_, err := client.ListVariables(context.Background(), ListVariablesOptions{State: "set"})
			return err
		},
		"padded project filter": func() error {
			_, err := client.ListVariables(context.Background(), ListVariablesOptions{ProjectID: " project"})
			return err
		},
		"empty create key": func() error {
			return client.CreateVariable(context.Background(), VariableRequest{Key: " ", Value: "allowed"})
		},
		"padded create key": func() error {
			return client.CreateVariable(context.Background(), VariableRequest{Key: "KEY ", Value: "allowed"})
		},
		"padded write project": func() error {
			return client.CreateVariable(context.Background(), VariableRequest{Key: "KEY", Value: "allowed", ProjectID: " project"})
		},
		"empty update ID": func() error {
			return client.UpdateVariable(context.Background(), "", VariableRequest{Key: "KEY", Value: "allowed"})
		},
		"padded delete ID": func() error { return client.DeleteVariable(context.Background(), "id ") },
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: call succeeded, want a validation error", name)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("%d invalid requests reached the server", len(server.requests))
	}
}
