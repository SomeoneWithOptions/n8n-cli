package n8n

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

const userResponse = `{
  "id":"123e4567-e89b-12d3-a456-426614174000",
  "email":"person@example.com",
  "firstName":"Pat",
  "lastName":"Example",
  "isPending":false,
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z",
  "role":"global:member",
  "mfaEnabled":true,
  "fieldAddedLater":"ignored"
}`

func TestListUsersQueryAuthAndDecoding(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[`+userResponse+`],"nextCursor":"next/one?x=1"}`))
	page, err := server.client(t).ListUsers(context.Background(), ListUsersOptions{
		ListOptions: ListOptions{Limit: 50}, Offset: 25, IncludeRole: true, ProjectID: "project/one?x=1",
	})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+UsersPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+UsersPath)
	}
	query := req.URL.Query()
	if query.Get("limit") != "50" || query.Get("offset") != "25" || query.Get("includeRole") != "true" || query.Get("projectId") != "project/one?x=1" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if len(page.Data) != 1 || page.Data[0].Email != "person@example.com" || page.Data[0].Role != "global:member" || !page.Data[0].MFAEnabled || page.NextCursor != "next/one?x=1" {
		t.Errorf("page = %+v", page)
	}
}

func TestListUsersCursorAndDefaultQuery(t *testing.T) {
	tests := []struct {
		name string
		opts ListUsersOptions
		want string
	}{
		{name: "defaults", opts: ListUsersOptions{}, want: ""},
		{name: "cursor", opts: ListUsersOptions{ListOptions: ListOptions{Cursor: "next/one?x=1"}}, want: "cursor=next%2Fone%3Fx%3D1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))
			if _, err := server.client(t).ListUsers(context.Background(), tt.opts); err != nil {
				t.Fatalf("ListUsers: %v", err)
			}
			if got := server.requests[0].URL.RawQuery; got != tt.want {
				t.Errorf("query = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateUsersPreservesPartialResults(t *testing.T) {
	response := `[
	  {"user":{"id":"one","email":"one@example.com","inviteAcceptUrl":"https://n8n.example/signup?token=secret","emailSent":false,"role":"global:member"},"error":""},
	  {"user":{"id":"two","email":"two@example.com","emailSent":false,"role":"global:member"},"error":"SMTP unavailable","future":"ignored"}
	]`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, response))
	requests := []CreateUserRequest{{Email: "one@example.com", Role: "global:member"}, {Email: "two@example.com"}}
	results, err := server.client(t).CreateUsers(context.Background(), requests)
	if err != nil {
		t.Fatalf("CreateUsers: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+UsersPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	assertJSONEqual(t, `[{"email":"one@example.com","role":"global:member"},{"email":"two@example.com"}]`, body)
	if len(results) != 2 || results[0].User == nil || results[0].User.InviteAcceptURL == "" || results[0].Error != "" || results[1].User == nil || results[1].Error != "SMTP unavailable" {
		t.Errorf("results = %+v", results)
	}
}

func TestGetChangeRoleAndDeleteUser(t *testing.T) {
	identifier := "person+ops@example.com/one?x=1"
	tests := []struct {
		name   string
		call   func(*Client) error
		method string
		path   string
		query  string
		body   string
		status int
	}{
		{name: "get", call: func(c *Client) error {
			user, err := c.GetUser(context.Background(), identifier, true)
			if err == nil && (user.Email != "person@example.com" || user.Role != "global:member") {
				t.Errorf("user = %+v", user)
			}
			return err
		}, method: http.MethodGet, path: "/users/person+ops@example.com%2Fone%3Fx=1", query: "includeRole=true"},
		{name: "change role", call: func(c *Client) error {
			return c.ChangeUserRole(context.Background(), identifier, "global:custom/role")
		}, method: http.MethodPatch, path: "/users/person+ops@example.com%2Fone%3Fx=1/role", body: `{"newRoleName":"global:custom/role"}`, status: http.StatusNoContent},
		{name: "delete", call: func(c *Client) error {
			return c.DeleteUser(context.Background(), identifier)
		}, method: http.MethodDelete, path: "/users/person+ops@example.com%2Fone%3Fx=1", status: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.status
			body := ""
			if status == 0 {
				status = http.StatusOK
				body = userResponse
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

func TestUserErrorsRemainStructured(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusForbidden, `{"message":"Only the owner can perform this action"}`))
	_, err := server.client(t).GetUser(context.Background(), "person@example.com", false)
	if StatusCodeOf(err) != http.StatusForbidden || !strings.Contains(err.Error(), "Only the owner") {
		t.Errorf("error = %v, want structured owner 403", err)
	}
}

func TestUserValidationBeforeRequest(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{}`))
	client := server.client(t)
	calls := map[string]func() error{
		"negative offset": func() error {
			_, err := client.ListUsers(context.Background(), ListUsersOptions{Offset: -1})
			return err
		},
		"large limit": func() error {
			_, err := client.ListUsers(context.Background(), ListUsersOptions{ListOptions: ListOptions{Limit: 251}})
			return err
		},
		"offset with cursor": func() error {
			_, err := client.ListUsers(context.Background(), ListUsersOptions{ListOptions: ListOptions{Cursor: "next"}, Offset: 1})
			return err
		},
		"empty invitations": func() error {
			_, err := client.CreateUsers(context.Background(), nil)
			return err
		},
		"invalid email": func() error {
			_, err := client.CreateUsers(context.Background(), []CreateUserRequest{{Email: "not-an-email"}})
			return err
		},
		"padded role": func() error {
			_, err := client.CreateUsers(context.Background(), []CreateUserRequest{{Email: "one@example.com", Role: " global:member"}})
			return err
		},
		"blank get identifier": func() error {
			_, err := client.GetUser(context.Background(), " ", false)
			return err
		},
		"blank changed role": func() error {
			return client.ChangeUserRole(context.Background(), "one@example.com", " ")
		},
		"padded delete identifier": func() error {
			return client.DeleteUser(context.Background(), " one@example.com")
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
