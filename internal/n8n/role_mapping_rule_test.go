package n8n

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const roleMappingRuleResponse = `{
  "id":"rule/one?x=1",
  "expression":"groups contains \"admins\"",
  "role":"global:admin",
  "type":"instance",
  "order":2,
  "projectIds":[],
  "createdAt":"2026-01-01T00:00:00.000Z",
  "updatedAt":"2026-01-02T00:00:00.000Z",
  "extraFieldAddedLater":"ignored"
}`

var wantRoleMappingRule = RoleMappingRule{
	ID:         "rule/one?x=1",
	Expression: `groups contains "admins"`,
	Role:       "global:admin",
	Type:       RoleMappingRuleTypeInstance,
	Order:      2,
	ProjectIDs: []string{},
	CreatedAt:  "2026-01-01T00:00:00.000Z",
	UpdatedAt:  "2026-01-02T00:00:00.000Z",
}

func TestListRoleMappingRules(t *testing.T) {
	body := `{"data":[` + roleMappingRuleResponse + `],"nextCursor":"next page"}`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))

	page, err := server.client(t).ListRoleMappingRules(context.Background(), ListRoleMappingRulesOptions{
		ListOptions: ListOptions{Limit: 25, Cursor: "start here"},
		Type:        RoleMappingRuleTypeProject,
	})
	if err != nil {
		t.Fatalf("ListRoleMappingRules: %v", err)
	}
	req, requestBody := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+RoleMappingRulesPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+RoleMappingRulesPath)
	}
	query := req.URL.Query()
	if query.Get("limit") != "25" || query.Get("cursor") != "start here" || query.Get("type") != "project" {
		t.Errorf("query = %q, want pagination and type", req.URL.RawQuery)
	}
	if requestBody != "" {
		t.Errorf("body = %q, want empty", requestBody)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if diff := cmp.Diff([]RoleMappingRule{wantRoleMappingRule}, page.Data); diff != "" {
		t.Errorf("page data mismatch (-want +got):\n%s", diff)
	}
	if page.NextCursor != "next page" || !page.HasMore() {
		t.Errorf("nextCursor = %q, HasMore = %v", page.NextCursor, page.HasMore())
	}
}

func TestListRoleMappingRulesDefaultsSendNoQuery(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))

	page, err := server.client(t).ListRoleMappingRules(context.Background(), ListRoleMappingRulesOptions{})
	if err != nil {
		t.Fatalf("ListRoleMappingRules: %v", err)
	}
	if len(page.Data) != 0 || page.HasMore() {
		t.Errorf("page = %+v, want an empty final page", page)
	}
	if req, _ := server.last(t); req.URL.RawQuery != "" {
		t.Errorf("query = %q, want empty", req.URL.RawQuery)
	}
}

func TestListRoleMappingRulesCollectsEveryPageKeepingTheFilter(t *testing.T) {
	pages := []string{
		`{"data":[{"id":"1","type":"project","order":0,"projectIds":["p1"]}],"nextCursor":"page two"}`,
		`{"data":[{"id":"2","type":"project","order":1,"projectIds":["p2"]}],"nextCursor":null}`,
	}
	var n int
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "project" {
			t.Errorf("request %d lost the type filter: %q", n+1, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", contentTypeJSON)
		_, _ = w.Write([]byte(pages[n]))
		n++
	})
	client := server.client(t)
	base := ListRoleMappingRulesOptions{Type: RoleMappingRuleTypeProject}
	fetch := func(ctx context.Context, pagination ListOptions) (Page[RoleMappingRule], error) {
		opts := base
		opts.ListOptions = pagination
		return client.ListRoleMappingRules(ctx, opts)
	}

	rules, err := Collect(context.Background(), fetch, ListOptions{}, 0)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(rules) != 2 || rules[0].ID != "1" || rules[1].ID != "2" {
		t.Errorf("rules = %+v, want both pages in order", rules)
	}
	if n != 2 {
		t.Errorf("requests = %d, want 2", n)
	}
}

func TestListRoleMappingRulesRejectsInvalidOptions(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))
	client := server.client(t)

	for name, opts := range map[string]ListRoleMappingRulesOptions{
		"negative limit": {ListOptions: ListOptions{Limit: -1}},
		"unknown type":   {Type: "global"},
	} {
		if _, err := client.ListRoleMappingRules(context.Background(), opts); err == nil {
			t.Errorf("%s: ListRoleMappingRules succeeded, want validation error", name)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want validation before transport", len(server.requests))
	}
}

func TestCreateRoleMappingRule(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, roleMappingRuleResponse))
	order := 0

	rule, err := server.client(t).CreateRoleMappingRule(context.Background(), CreateRoleMappingRuleRequest{
		Expression: `groups contains "admins"`,
		Role:       "project:admin",
		Type:       RoleMappingRuleTypeProject,
		Order:      &order,
		ProjectIDs: []string{"project-one"},
	})
	if err != nil {
		t.Fatalf("CreateRoleMappingRule: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+RoleMappingRulesPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.Path, BasePath+RoleMappingRulesPath)
	}
	want := `{"expression":"groups contains \"admins\"","role":"project:admin","type":"project","order":0,"projectIds":["project-one"]}`
	if strings.TrimSpace(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}
	if diff := cmp.Diff(&wantRoleMappingRule, rule); diff != "" {
		t.Errorf("rule mismatch (-want +got):\n%s", diff)
	}
}

func TestCreateRoleMappingRuleOmitsUnsetOrderAndProjects(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, roleMappingRuleResponse))

	if _, err := server.client(t).CreateRoleMappingRule(context.Background(), CreateRoleMappingRuleRequest{
		Expression: "email endsWith \"@example.com\"",
		Role:       "global:member",
		Type:       RoleMappingRuleTypeInstance,
	}); err != nil {
		t.Fatalf("CreateRoleMappingRule: %v", err)
	}
	_, body := server.last(t)
	if strings.Contains(body, "order") || strings.Contains(body, "projectIds") {
		t.Errorf("body = %s, want order and projectIds omitted so the rule is appended", body)
	}
}

func TestCreateRoleMappingRuleValidatesBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, roleMappingRuleResponse))
	client := server.client(t)
	negative := -1

	for name, request := range map[string]CreateRoleMappingRuleRequest{
		"empty expression": {Expression: "  ", Role: "global:admin", Type: RoleMappingRuleTypeInstance},
		"empty role":       {Expression: "true", Role: "", Type: RoleMappingRuleTypeInstance},
		"long role": {Expression: "true", Type: RoleMappingRuleTypeInstance,
			Role: strings.Repeat("r", maxRoleMappingRoleLength+1)},
		"padded role":  {Expression: "true", Role: " global:admin", Type: RoleMappingRuleTypeInstance},
		"unknown type": {Expression: "true", Role: "global:admin", Type: "global"},
		"negative order": {Expression: "true", Role: "global:admin",
			Type: RoleMappingRuleTypeInstance, Order: &negative},
		"empty project ID": {Expression: "true", Role: "project:admin",
			Type: RoleMappingRuleTypeProject, ProjectIDs: []string{" "}},
		"projects on an instance rule": {Expression: "true", Role: "global:admin",
			Type: RoleMappingRuleTypeInstance, ProjectIDs: []string{"project-one"}},
	} {
		if _, err := client.CreateRoleMappingRule(context.Background(), request); err == nil {
			t.Errorf("%s: CreateRoleMappingRule succeeded, want validation error", name)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want validation before transport", len(server.requests))
	}
}

func TestMoveRoleMappingRuleEscapesID(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, roleMappingRuleResponse))

	rule, err := server.client(t).MoveRoleMappingRule(context.Background(), "rule/one?x=1",
		MoveRoleMappingRuleRequest{TargetIndex: 3})
	if err != nil {
		t.Fatalf("MoveRoleMappingRule: %v", err)
	}
	req, body := server.last(t)
	wantPath := BasePath + "/role-mapping-rules/rule%2Fone%3Fx=1/move"
	if req.Method != http.MethodPost || req.URL.EscapedPath() != wantPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.EscapedPath(), wantPath)
	}
	if strings.TrimSpace(body) != `{"targetIndex":3}` {
		t.Errorf("body = %s, want the target index", body)
	}
	if rule.ID != wantRoleMappingRule.ID {
		t.Errorf("rule = %+v, want the moved rule", rule)
	}
}

func TestMoveRoleMappingRuleValidatesBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, roleMappingRuleResponse))
	client := server.client(t)

	if _, err := client.MoveRoleMappingRule(context.Background(), " ", MoveRoleMappingRuleRequest{}); err == nil {
		t.Error("empty ID: MoveRoleMappingRule succeeded, want validation error")
	}
	if _, err := client.MoveRoleMappingRule(context.Background(), "rule-1", MoveRoleMappingRuleRequest{TargetIndex: -1}); err == nil {
		t.Error("negative index: MoveRoleMappingRule succeeded, want validation error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want validation before transport", len(server.requests))
	}
}

func TestUpdateRoleMappingRuleSendsOnlyPresentFields(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, roleMappingRuleResponse))
	role := "global:member"

	if _, err := server.client(t).UpdateRoleMappingRule(context.Background(), "rule/one?x=1",
		UpdateRoleMappingRuleRequest{Role: &role}); err != nil {
		t.Fatalf("UpdateRoleMappingRule: %v", err)
	}
	req, body := server.last(t)
	wantPath := BasePath + "/role-mapping-rules/rule%2Fone%3Fx=1"
	if req.Method != http.MethodPatch || req.URL.EscapedPath() != wantPath {
		t.Errorf("request = %s %s, want PATCH %s", req.Method, req.URL.EscapedPath(), wantPath)
	}
	if strings.TrimSpace(body) != `{"role":"global:member"}` {
		t.Errorf("body = %s, want only the role", body)
	}
}

func TestUpdateRoleMappingRuleKeepsExplicitEmptyProjectList(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, roleMappingRuleResponse))
	none := []string{}

	if _, err := server.client(t).UpdateRoleMappingRule(context.Background(), "rule-1",
		UpdateRoleMappingRuleRequest{ProjectIDs: &none}); err != nil {
		t.Fatalf("UpdateRoleMappingRule: %v", err)
	}
	if _, body := server.last(t); strings.TrimSpace(body) != `{"projectIds":[]}` {
		t.Errorf("body = %s, want an explicit empty array that clears the projects", body)
	}
}

func TestUpdateRoleMappingRuleValidatesBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, roleMappingRuleResponse))
	client := server.client(t)
	empty := ""
	long := strings.Repeat("r", maxRoleMappingRoleLength+1)
	ids := []string{""}

	for name, request := range map[string]UpdateRoleMappingRuleRequest{
		"no field":         {},
		"empty expression": {Expression: &empty},
		"long role":        {Role: &long},
		"empty project ID": {ProjectIDs: &ids},
	} {
		if _, err := client.UpdateRoleMappingRule(context.Background(), "rule-1", request); err == nil {
			t.Errorf("%s: UpdateRoleMappingRule succeeded, want validation error", name)
		}
	}
	if _, err := client.UpdateRoleMappingRule(context.Background(), "", UpdateRoleMappingRuleRequest{Role: &long}); err == nil {
		t.Error("empty ID: UpdateRoleMappingRule succeeded, want validation error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want validation before transport", len(server.requests))
	}
}

func TestDeleteRoleMappingRuleReturnsTheDeletedRule(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, roleMappingRuleResponse))

	rule, err := server.client(t).DeleteRoleMappingRule(context.Background(), "rule/one?x=1")
	if err != nil {
		t.Fatalf("DeleteRoleMappingRule: %v", err)
	}
	req, body := server.last(t)
	wantPath := BasePath + "/role-mapping-rules/rule%2Fone%3Fx=1"
	if req.Method != http.MethodDelete || req.URL.EscapedPath() != wantPath {
		t.Errorf("request = %s %s, want DELETE %s", req.Method, req.URL.EscapedPath(), wantPath)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if diff := cmp.Diff(&wantRoleMappingRule, rule); diff != "" {
		t.Errorf("rule mismatch (-want +got):\n%s", diff)
	}
	if _, err := server.client(t).DeleteRoleMappingRule(context.Background(), " "); err == nil {
		t.Error("empty ID: DeleteRoleMappingRule succeeded, want validation error")
	}
}

func TestRoleMappingRuleErrorsAreReported(t *testing.T) {
	for name, status := range map[string]int{
		"bad request": http.StatusBadRequest,
		"unauthentic": http.StatusUnauthorized,
		"forbidden":   http.StatusForbidden,
		"not found":   http.StatusNotFound,
	} {
		server := newCommunityPackageServer(t, packageJSONHandler(status, `{"message":"no"}`))
		client := server.client(t)

		if _, err := client.ListRoleMappingRules(context.Background(), ListRoleMappingRulesOptions{}); !IsStatus(err, status) {
			t.Errorf("%s: list error = %v, want status %d", name, err, status)
		}
		if _, err := client.DeleteRoleMappingRule(context.Background(), "rule-1"); !IsStatus(err, status) {
			t.Errorf("%s: delete error = %v, want status %d", name, err, status)
		}
	}
}

func TestRoleMappingRuleHandlesEmptyResponseBody(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rule, err := server.client(t).DeleteRoleMappingRule(context.Background(), "rule-1")
	if err != nil {
		t.Fatalf("DeleteRoleMappingRule: %v", err)
	}
	if diff := cmp.Diff(&RoleMappingRule{}, rule); diff != "" {
		t.Errorf("rule mismatch (-want +got):\n%s", diff)
	}
}
