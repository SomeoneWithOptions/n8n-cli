package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const roleResponse = `{
  "slug":"global:custom/one?x=1",
  "displayName":"Workflow auditor",
  "description":"Can inspect workflows",
  "systemRole":false,
  "roleType":"global",
  "scopes":["workflow:list","workflow:read"],
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z",
  "licensed":false,
  "usedByUsers":0,
  "usedByProjects":2,
  "extraFieldAddedLater":"ignored"
}`

func TestListRoles(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"global":[`+roleResponse+`],"project":[]}`))
	roles, err := server.client(t).ListRoles(context.Background(), true)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+RolesPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+RolesPath)
	}
	if got := req.URL.Query().Get("withUsageCount"); got != "true" {
		t.Errorf("withUsageCount = %q, want true", got)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if len(roles.Global) != 1 || len(roles.Project) != 0 {
		t.Fatalf("roles = %+v", roles)
	}
	role := roles.Global[0]
	if role.Slug != "global:custom/one?x=1" || role.Licensed == nil || *role.Licensed ||
		role.UsedByUsers == nil || *role.UsedByUsers != 0 || role.UsedByProjects == nil || *role.UsedByProjects != 2 {
		t.Errorf("decoded role = %+v", role)
	}
}

func TestListRolesOmitsDefaultUsageQuery(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"global":[],"project":[]}`))
	if _, err := server.client(t).ListRoles(context.Background(), false); err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if got := server.requests[0].URL.RawQuery; got != "" {
		t.Errorf("query = %q, want empty", got)
	}
}

func TestCreateGetUpdateAndDeleteRole(t *testing.T) {
	description := "Can inspect workflows"
	tests := []struct {
		name    string
		call    func(*Client) (*Role, error)
		method  string
		status  int
		escaped string
		query   string
		body    string
	}{
		{name: "create", call: func(c *Client) (*Role, error) {
			return c.CreateRole(context.Background(), CreateRoleRequest{
				DisplayName: "Workflow auditor", Description: &description, RoleType: RoleTypeGlobal,
				Scopes: []string{"workflow:list", "workflow:read"},
			})
		}, method: http.MethodPost, status: http.StatusCreated, escaped: RolesPath,
			body: `{"displayName":"Workflow auditor","description":"Can inspect workflows","roleType":"global","scopes":["workflow:list","workflow:read"]}`},
		{name: "get with usage", call: func(c *Client) (*Role, error) {
			return c.GetRole(context.Background(), "global:custom/one?x=1", true)
		}, method: http.MethodGet, escaped: "/roles/global:custom%2Fone%3Fx=1", query: "withUsageCount=true"},
		{name: "update with null description and empty scopes", call: func(c *Client) (*Role, error) {
			return c.UpdateRole(context.Background(), "global:custom/one?x=1", UpdateRoleRequest{
				DisplayName: "Workflow auditor", Description: NullRoleDescription(), Scopes: []string{},
			})
		}, method: http.MethodPut, escaped: "/roles/global:custom%2Fone%3Fx=1",
			body: `{"displayName":"Workflow auditor","description":null,"scopes":[]}`},
		{name: "delete and reassign", call: func(c *Client) (*Role, error) {
			return c.DeleteRole(context.Background(), "global:custom/one?x=1", "global:member/limited?x=2")
		}, method: http.MethodDelete, escaped: "/roles/global:custom%2Fone%3Fx=1", query: "reassignRoleSlug=global%3Amember%2Flimited%3Fx%3D2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.status
			if status == 0 {
				status = http.StatusOK
			}
			server := newCommunityPackageServer(t, packageJSONHandler(status, roleResponse))
			role, err := tt.call(server.client(t))
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			req, body := server.last(t)
			if req.Method != tt.method || req.URL.EscapedPath() != BasePath+tt.escaped {
				t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.EscapedPath(), tt.method, BasePath+tt.escaped)
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
			if role.Slug != "global:custom/one?x=1" || role.DisplayName != "Workflow auditor" {
				t.Errorf("role = %+v", role)
			}
		})
	}
}

func TestRoleDescriptionJSON(t *testing.T) {
	var request UpdateRoleRequest
	if err := json.Unmarshal([]byte(`{"displayName":"Auditor","description":"Read only","scopes":[]}`), &request); err != nil {
		t.Fatalf("unmarshal string description: %v", err)
	}
	if value, ok := request.Description.Value(); !ok || value != "Read only" {
		t.Errorf("description = %q, %v", value, ok)
	}
	if err := json.Unmarshal([]byte(`{"displayName":"Auditor","description":null,"scopes":[]}`), &request); err != nil {
		t.Fatalf("unmarshal null description: %v", err)
	}
	if !request.Description.IsNull() {
		t.Error("description is not explicit null")
	}
}

func TestRoleImmutableErrorsRemainStructured(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusBadRequest, `{"message":"System roles cannot be modified"}`))
			client := server.client(t)
			var err error
			if method == http.MethodPut {
				_, err = client.UpdateRole(context.Background(), "global:owner", UpdateRoleRequest{
					DisplayName: "Owner", Description: NullRoleDescription(), Scopes: []string{},
				})
			} else {
				_, err = client.DeleteRole(context.Background(), "global:owner", "")
			}
			if StatusCodeOf(err) != http.StatusBadRequest || !strings.Contains(err.Error(), "System roles") {
				t.Errorf("error = %v, want structured 400 with server message", err)
			}
		})
	}
}

func TestRoleValidationBeforeRequest(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, roleResponse))
	client := server.client(t)
	validCreate := CreateRoleRequest{DisplayName: "Auditor", RoleType: RoleTypeGlobal, Scopes: []string{}}
	validUpdate := UpdateRoleRequest{DisplayName: "Auditor", Description: NullRoleDescription(), Scopes: []string{}}
	calls := map[string]func() error{
		"blank create name": func() error {
			_, err := client.CreateRole(context.Background(), CreateRoleRequest{DisplayName: " ", RoleType: RoleTypeGlobal, Scopes: []string{}})
			return err
		},
		"short create name": func() error {
			_, err := client.CreateRole(context.Background(), CreateRoleRequest{DisplayName: "x", RoleType: RoleTypeGlobal, Scopes: []string{}})
			return err
		},
		"invalid role type": func() error {
			request := validCreate
			request.RoleType = "team"
			_, err := client.CreateRole(context.Background(), request)
			return err
		},
		"missing create scopes": func() error {
			request := validCreate
			request.Scopes = nil
			_, err := client.CreateRole(context.Background(), request)
			return err
		},
		"blank create scope": func() error {
			request := validCreate
			request.Scopes = []string{"workflow:read", " "}
			_, err := client.CreateRole(context.Background(), request)
			return err
		},
		"blank get slug": func() error {
			_, err := client.GetRole(context.Background(), " ", false)
			return err
		},
		"missing update description": func() error {
			request := validUpdate
			request.Description = RoleDescription{}
			_, err := client.UpdateRole(context.Background(), "custom", request)
			return err
		},
		"padded update slug": func() error {
			_, err := client.UpdateRole(context.Background(), " custom", validUpdate)
			return err
		},
		"blank reassignment slug": func() error {
			_, err := client.DeleteRole(context.Background(), "custom", " ")
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Error("call succeeded, want validation error")
			}
		})
	}
	if diff := cmp.Diff(0, len(server.requests)); diff != "" {
		t.Errorf("request count mismatch (-want +got):\n%s", diff)
	}
}
