package n8n

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

const projectResponse = `{
  "id":"pr-1",
  "name":"Billing automation",
  "type":"team",
  "fieldAddedLater":"ignored"
}`

const projectMemberResponse = `{
  "id":"123e4567-e89b-12d3-a456-426614174000",
  "email":"person@example.com",
  "firstName":"Pat",
  "lastName":"Example",
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z",
  "role":"project:viewer",
  "fieldAddedLater":"ignored"
}`

func TestListProjectsQueryAuthAndDecoding(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[`+projectResponse+`],"nextCursor":"next/one?x=1"}`))
	page, err := server.client(t).ListProjects(context.Background(), ListOptions{Limit: 50, Cursor: "prev/one?x=1"})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+ProjectsPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+ProjectsPath)
	}
	if query := req.URL.Query(); query.Get("limit") != "50" || query.Get("cursor") != "prev/one?x=1" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "pr-1" || page.Data[0].Name != "Billing automation" || page.Data[0].Type != "team" || page.NextCursor != "next/one?x=1" {
		t.Errorf("page = %+v", page)
	}
}

func TestListProjectsDefaultQuery(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))
	if _, err := server.client(t).ListProjects(context.Background(), ListOptions{}); err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if got := server.requests[0].URL.RawQuery; got != "" {
		t.Errorf("query = %q, want empty", got)
	}
}

func TestCreateProjectSendsNameAndToleratesEmptyBody(t *testing.T) {
	t.Run("body returned", func(t *testing.T) {
		server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, projectResponse))
		project, err := server.client(t).CreateProject(context.Background(), "Billing automation")
		if err != nil {
			t.Fatalf("CreateProject: %v", err)
		}
		req, body := server.last(t)
		if req.Method != http.MethodPost || req.URL.Path != BasePath+ProjectsPath {
			t.Errorf("request = %s %s", req.Method, req.URL.Path)
		}
		assertJSONEqual(t, `{"name":"Billing automation"}`, body)
		if project.ID != "pr-1" || project.Type != "team" {
			t.Errorf("project = %+v", project)
		}
	})

	t.Run("no body", func(t *testing.T) {
		server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})
		project, err := server.client(t).CreateProject(context.Background(), "Billing automation")
		if err != nil {
			t.Fatalf("CreateProject: %v", err)
		}
		if project == nil || project.ID != "" || project.Name != "" {
			t.Errorf("project = %+v, want empty project for an empty 201", project)
		}
	})
}

func TestProjectAndMemberRequests(t *testing.T) {
	projectID := "pr/one?x=1"
	userID := "us/two?y=2"
	tests := []struct {
		name   string
		call   func(*Client) error
		method string
		path   string
		query  string
		body   string
		status int
	}{
		{name: "update", call: func(c *Client) error {
			return c.UpdateProject(context.Background(), projectID, "Billing")
		}, method: http.MethodPut, path: "/projects/pr%2Fone%3Fx=1", body: `{"name":"Billing"}`, status: http.StatusNoContent},
		{name: "delete", call: func(c *Client) error {
			return c.DeleteProject(context.Background(), projectID)
		}, method: http.MethodDelete, path: "/projects/pr%2Fone%3Fx=1", status: http.StatusNoContent},
		{name: "member list", call: func(c *Client) error {
			page, err := c.ListProjectUsers(context.Background(), projectID, ListOptions{Limit: 10, Cursor: "next/one?x=1"})
			if err == nil && (len(page.Data) != 1 || page.Data[0].Role != "project:viewer" || page.Data[0].Email != "person@example.com") {
				t.Errorf("page = %+v", page)
			}
			return err
		}, method: http.MethodGet, path: "/projects/pr%2Fone%3Fx=1/users", query: "cursor=next%2Fone%3Fx%3D1&limit=10"},
		{name: "member add", call: func(c *Client) error {
			return c.AddUsersToProject(context.Background(), projectID, []ProjectRelation{
				{UserID: "one", Role: "project:viewer"},
				{UserID: "two", Role: "project:editor"},
			})
		}, method: http.MethodPost, path: "/projects/pr%2Fone%3Fx=1/users",
			body:   `{"relations":[{"userId":"one","role":"project:viewer"},{"userId":"two","role":"project:editor"}]}`,
			status: http.StatusCreated},
		{name: "member role change", call: func(c *Client) error {
			return c.ChangeUserRoleInProject(context.Background(), projectID, userID, "project:admin")
		}, method: http.MethodPatch, path: "/projects/pr%2Fone%3Fx=1/users/us%2Ftwo%3Fy=2", body: `{"role":"project:admin"}`, status: http.StatusNoContent},
		{name: "member remove", call: func(c *Client) error {
			return c.DeleteUserFromProject(context.Background(), projectID, userID)
		}, method: http.MethodDelete, path: "/projects/pr%2Fone%3Fx=1/users/us%2Ftwo%3Fy=2", status: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := tt.status, ""
			if status == 0 {
				status = http.StatusOK
				body = `{"data":[` + projectMemberResponse + `],"nextCursor":null}`
			}
			server := newCommunityPackageServer(t, packageJSONHandler(status, body))
			if err := tt.call(server.client(t)); err != nil {
				t.Fatalf("call: %v", err)
			}
			req, gotBody := server.last(t)
			if req.Method != tt.method || req.URL.EscapedPath() != BasePath+tt.path {
				t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.EscapedPath(), tt.method, BasePath+tt.path)
			}
			if req.URL.RawQuery != tt.query {
				t.Errorf("query = %q, want %q", req.URL.RawQuery, tt.query)
			}
			if tt.body == "" {
				if gotBody != "" {
					t.Errorf("body = %q, want empty", gotBody)
				}
			} else {
				assertJSONEqual(t, tt.body, gotBody)
			}
		})
	}
}

func TestProjectPaginationCollectsEveryPage(t *testing.T) {
	pages := 0
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		if pages == 0 {
			pages++
			_, _ = w.Write([]byte(`{"data":[` + projectResponse + `],"nextCursor":"second"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"pr-2","name":"Second","type":"team"}],"nextCursor":null}`))
	})
	client := server.client(t)
	projects, err := Collect(context.Background(), client.ListProjects, ListOptions{Limit: 1}, 0)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(projects) != 2 || projects[1].ID != "pr-2" {
		t.Errorf("projects = %+v, want both pages", projects)
	}
	if got := server.requests[1].URL.Query().Get("cursor"); got != "second" {
		t.Errorf("second cursor = %q, want %q", got, "second")
	}
}

func TestProjectErrorsRemainStructured(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusForbidden,
		`{"message":"Your license does not allow for feat:projectRole:admin."}`))
	_, err := server.client(t).ListProjects(context.Background(), ListOptions{})
	if StatusCodeOf(err) != http.StatusForbidden || !strings.Contains(err.Error(), "license") {
		t.Errorf("error = %v, want structured licensing 403", err)
	}
}

func TestProjectValidationBeforeRequest(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{}`))
	client := server.client(t)
	calls := map[string]func() error{
		"negative limit": func() error {
			_, err := client.ListProjects(context.Background(), ListOptions{Limit: -1})
			return err
		},
		"large limit": func() error {
			_, err := client.ListProjects(context.Background(), ListOptions{Limit: 251})
			return err
		},
		"blank create name": func() error {
			_, err := client.CreateProject(context.Background(), " ")
			return err
		},
		"padded update name": func() error {
			return client.UpdateProject(context.Background(), "pr-1", "Billing ")
		},
		"blank update ID": func() error {
			return client.UpdateProject(context.Background(), "", "Billing")
		},
		"padded delete ID": func() error {
			return client.DeleteProject(context.Background(), " pr-1")
		},
		"member list blank project": func() error {
			_, err := client.ListProjectUsers(context.Background(), " ", ListOptions{})
			return err
		},
		"member list large limit": func() error {
			_, err := client.ListProjectUsers(context.Background(), "pr-1", ListOptions{Limit: 251})
			return err
		},
		"no relations": func() error {
			return client.AddUsersToProject(context.Background(), "pr-1", nil)
		},
		"relation without role": func() error {
			return client.AddUsersToProject(context.Background(), "pr-1", []ProjectRelation{{UserID: "one"}})
		},
		"relation with padded user": func() error {
			return client.AddUsersToProject(context.Background(), "pr-1", []ProjectRelation{{UserID: " one", Role: "project:viewer"}})
		},
		"blank changed role": func() error {
			return client.ChangeUserRoleInProject(context.Background(), "pr-1", "us-1", " ")
		},
		"blank removed user": func() error {
			return client.DeleteUserFromProject(context.Background(), "pr-1", "")
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Error("call succeeded, want validation error")
			}
		})
	}
	if len(server.requests) != 0 {
		t.Errorf("request count = %d, want 0", len(server.requests))
	}
}
