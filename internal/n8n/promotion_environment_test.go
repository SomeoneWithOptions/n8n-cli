package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const testEnvironmentID = "0b8f7a2e-4c1d-4e5f-9a6b-7c8d9e0f1a2b"

const promotionApplyEnvironmentBody = `{
  "id":"` + testEnvironmentID + `",
  "name":"prod",
  "scope":"instance",
  "remote":{"schemaVersion":1,"remoteUrl":"https://gitlab.example.com/org/repo.git","repositoryPath":"org/repo"},
  "provider":{"id":"prov-1","name":"gitlab","type":"gitlab","authType":"token","createdAt":"2026-10-01T00:00:00.000Z","updatedAt":"2026-10-01T00:00:00.000Z"},
  "createdAt":"2026-10-01T00:00:00.000Z",
  "updatedAt":"2026-10-01T00:00:00.000Z",
  "direction":"apply",
  "settings":{"schemaVersion":1,"branchName":"main"},
  "checkout":{"hasCheckout":true,"matchesEnvironment":false}
}`

const promotionPromoteEnvironmentBody = `{
  "id":"` + testEnvironmentID + `",
  "name":"prod-push",
  "scope":"projects",
  "remote":{"schemaVersion":1,"remoteUrl":"git@example.com:org/repo.git"},
  "provider":{"id":"prov-1","name":"github","type":"git","authType":"ssh-key","createdAt":"2026-10-01T00:00:00.000Z","updatedAt":"2026-10-01T00:00:00.000Z"},
  "createdAt":"2026-10-01T00:00:00.000Z",
  "updatedAt":"2026-10-01T00:00:00.000Z",
  "direction":"promote",
  "settings":{"schemaVersion":1,"baseBranchName":"main","createBranchOnPromotion":false}
}`

const promotionEnvironmentCheckoutBody = `{
  "environmentId":"` + testEnvironmentID + `",
  "direction":"apply",
  "branchName":"main",
  "hasCheckout":true
}`

func boolPtr(v bool) *bool { return &v }

func validApplyEnvironment() CreatePromotionEnvironmentRequest {
	return CreatePromotionEnvironmentRequest{
		Name: "prod", Scope: PromotionScopeInstance, ProviderID: "prov-1",
		Remote:    PromotionRemoteDocument{SchemaVersion: 1, RemoteURL: "git@example.com:org/repo.git"},
		Direction: PromotionDirectionApply,
		Settings:  PromotionEnvironmentSettings{SchemaVersion: 1, BranchName: "main"},
	}
}

func TestListPromotionEnvironments(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[`+promotionApplyEnvironmentBody+`,`+promotionPromoteEnvironmentBody+`],"nextCursor":"next"}`))

	page, err := server.client(t).ListPromotionEnvironments(context.Background(), ListPromotionEnvironmentsOptions{
		ListOptions: ListOptions{Limit: 10, Cursor: "c1"},
		Scope:       PromotionScopeInstance,
		Direction:   PromotionDirectionApply,
		ProviderID:  "prov-1",
	})
	if err != nil {
		t.Fatalf("ListPromotionEnvironments: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+PromotionEnvironmentsPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+PromotionEnvironmentsPath)
	}
	query := req.URL.Query()
	if query.Get("limit") != "10" || query.Get("cursor") != "c1" || query.Get("scope") != "instance" ||
		query.Get("direction") != "apply" || query.Get("providerId") != "prov-1" {
		t.Errorf("query = %q, want limit, cursor, scope, direction and providerId", req.URL.RawQuery)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if len(page.Data) != 2 || page.NextCursor != "next" {
		t.Fatalf("page = %+v, want two environments with a cursor", page)
	}
	apply, promote := page.Data[0], page.Data[1]
	if apply.Direction != PromotionDirectionApply || apply.Settings.BranchName != "main" || apply.Remote.RepositoryPath != "org/repo" {
		t.Errorf("apply environment = %+v", apply)
	}
	if apply.Checkout == nil || !apply.Checkout.HasCheckout || apply.Checkout.MatchesEnvironment {
		t.Errorf("apply checkout = %+v, want a stale checkout", apply.Checkout)
	}
	if promote.Direction != PromotionDirectionPromote || promote.Settings.BaseBranchName != "main" ||
		promote.Settings.CreateBranchOnPromotion == nil || *promote.Settings.CreateBranchOnPromotion || promote.Checkout != nil {
		t.Errorf("promote environment = %+v", promote)
	}
}

func TestListPromotionEnvironmentsValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))
	client := server.client(t)

	cases := []ListPromotionEnvironmentsOptions{
		{ListOptions: ListOptions{Limit: 251}},
		{Scope: "global"},
		{Direction: "sideways"},
		{ProviderID: "  "},
	}
	for i, opts := range cases {
		if _, err := client.ListPromotionEnvironments(context.Background(), opts); err == nil {
			t.Errorf("case %d: want a validation error", i)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestCreatePromotionEnvironment(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, promotionPromoteEnvironmentBody))

	request := CreatePromotionEnvironmentRequest{
		Name: "prod-push", Scope: PromotionScopeProjects, ProviderID: "prov-1",
		Remote:    PromotionRemoteDocument{SchemaVersion: 1, RemoteURL: "git@example.com:org/repo.git"},
		Direction: PromotionDirectionPromote,
		Settings:  PromotionEnvironmentSettings{SchemaVersion: 1, BaseBranchName: "main", CreateBranchOnPromotion: boolPtr(false)},
	}
	created, err := server.client(t).CreatePromotionEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("CreatePromotionEnvironment: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+PromotionEnvironmentsPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.Path, BasePath+PromotionEnvironmentsPath)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	if sent["direction"] != "promote" || sent["providerId"] != "prov-1" || sent["scope"] != "projects" {
		t.Errorf("request = %v, want direction, providerId and scope", sent)
	}
	settings, _ := sent["settings"].(map[string]any)
	if _, ok := settings["branchName"]; ok || settings["createBranchOnPromotion"] != false || settings["baseBranchName"] != "main" {
		t.Errorf("settings = %v, want only the promote shape with an explicit false", settings)
	}
	if strings.Contains(raw, "repositoryPath") || strings.Contains(raw, `"id"`) || strings.Contains(raw, "checkout") {
		t.Errorf("request carries a read-only field: %s", raw)
	}
	if created.ID != testEnvironmentID || created.Direction != PromotionDirectionPromote {
		t.Errorf("created = %+v", created)
	}
}

func TestCreatePromotionEnvironmentValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, promotionApplyEnvironmentBody))
	client := server.client(t)

	mutate := func(change func(*CreatePromotionEnvironmentRequest)) CreatePromotionEnvironmentRequest {
		request := validApplyEnvironment()
		change(&request)
		return request
	}
	cases := map[string]CreatePromotionEnvironmentRequest{
		"empty":          {},
		"blank name":     mutate(func(r *CreatePromotionEnvironmentRequest) { r.Name = " " }),
		"bad scope":      mutate(func(r *CreatePromotionEnvironmentRequest) { r.Scope = "global" }),
		"no provider":    mutate(func(r *CreatePromotionEnvironmentRequest) { r.ProviderID = "" }),
		"remote version": mutate(func(r *CreatePromotionEnvironmentRequest) { r.Remote.SchemaVersion = 2 }),
		"remote url":     mutate(func(r *CreatePromotionEnvironmentRequest) { r.Remote.RemoteURL = " " }),
		"bad direction":  mutate(func(r *CreatePromotionEnvironmentRequest) { r.Direction = "sideways" }),
		"mismatched": mutate(func(r *CreatePromotionEnvironmentRequest) {
			r.Direction = PromotionDirectionPromote
		}),
		"mixed settings": mutate(func(r *CreatePromotionEnvironmentRequest) { r.Settings.BaseBranchName = "main" }),
		"blank branch":   mutate(func(r *CreatePromotionEnvironmentRequest) { r.Settings.BranchName = " " }),
		"settings version": mutate(func(r *CreatePromotionEnvironmentRequest) {
			r.Settings.SchemaVersion = 0
		}),
		"promote without create flag": mutate(func(r *CreatePromotionEnvironmentRequest) {
			r.Direction = PromotionDirectionPromote
			r.Settings = PromotionEnvironmentSettings{SchemaVersion: 1, BaseBranchName: "main"}
		}),
	}
	for name, request := range cases {
		if _, err := client.CreatePromotionEnvironment(context.Background(), request); err == nil {
			t.Errorf("%s: want a validation error", name)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestGetPromotionEnvironment(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionApplyEnvironmentBody))

	environment, err := server.client(t).GetPromotionEnvironment(context.Background(), testEnvironmentID)
	if err != nil {
		t.Fatalf("GetPromotionEnvironment: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+PromotionEnvironmentsPath+"/"+testEnvironmentID {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if environment.Name != "prod" || environment.Provider.Type != PromotionProviderTypeGitLab {
		t.Errorf("environment = %+v", environment)
	}
}

func TestPromotionEnvironmentIDValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionApplyEnvironmentBody))
	client := server.client(t)
	ctx := context.Background()

	for _, id := range []string{"", " " + testEnvironmentID, "env-1", "a/b", testEnvironmentID + "0"} {
		if _, err := client.GetPromotionEnvironment(ctx, id); err == nil {
			t.Errorf("get %q: want a validation error", id)
		}
		if err := client.DeletePromotionEnvironment(ctx, id); err == nil {
			t.Errorf("delete %q: want a validation error", id)
		}
		if _, err := client.ConnectPromotionEnvironment(ctx, id); err == nil {
			t.Errorf("connect %q: want a validation error", id)
		}
		if _, err := client.ListPromotionEnvironmentProjects(ctx, id); err == nil {
			t.Errorf("projects %q: want a validation error", id)
		}
	}
	if _, err := client.AddProjectToPromotionEnvironment(ctx, testEnvironmentID, " "); err == nil {
		t.Error("blank project ID: want a validation error")
	}
	if err := client.RemoveProjectFromPromotionEnvironment(ctx, testEnvironmentID, ""); err == nil {
		t.Error("empty project ID: want a validation error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
	if err := ValidatePromotionEnvironmentID(strings.ToUpper(testEnvironmentID)); err != nil {
		t.Errorf("upper-case UUID: %v", err)
	}
}

func TestUpdatePromotionEnvironment(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionApplyEnvironmentBody))

	updated, err := server.client(t).UpdatePromotionEnvironment(context.Background(), testEnvironmentID, UpdatePromotionEnvironmentRequest{
		Name:     strPtr("staging"),
		Settings: &PromotionEnvironmentSettings{SchemaVersion: 1, BranchName: "release"},
	})
	if err != nil {
		t.Fatalf("UpdatePromotionEnvironment: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPut || req.URL.Path != BasePath+PromotionEnvironmentsPath+"/"+testEnvironmentID {
		t.Errorf("request = %s %s, want PUT", req.Method, req.URL.Path)
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("body = %q (error %v)", raw, err)
	}
	if string(sent["name"]) != `"staging"` || string(sent["settings"]) != `{"schemaVersion":1,"branchName":"release"}` || len(sent) != 2 {
		t.Errorf("body = %s, want only name and settings", raw)
	}
	if updated.ID != testEnvironmentID {
		t.Errorf("updated = %+v", updated)
	}
}

func TestUpdatePromotionEnvironmentValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionApplyEnvironmentBody))
	client := server.client(t)

	cases := map[string]UpdatePromotionEnvironmentRequest{
		"empty":          {},
		"blank name":     {Name: strPtr("")},
		"blank provider": {ProviderID: strPtr(" ")},
		"bad remote":     {Remote: &PromotionRemoteDocument{SchemaVersion: 9, RemoteURL: "x"}},
		"mixed settings": {Settings: &PromotionEnvironmentSettings{SchemaVersion: 1, BranchName: "a", BaseBranchName: "b"}},
		"no settings":    {Settings: &PromotionEnvironmentSettings{SchemaVersion: 1}},
	}
	for name, request := range cases {
		if _, err := client.UpdatePromotionEnvironment(context.Background(), testEnvironmentID, request); err == nil {
			t.Errorf("%s: want a validation error", name)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestPromotionEnvironmentSettingsValidateDirection(t *testing.T) {
	apply := PromotionEnvironmentSettings{SchemaVersion: 1, BranchName: "main"}
	promote := PromotionEnvironmentSettings{SchemaVersion: 1, BaseBranchName: "main", CreateBranchOnPromotion: boolPtr(true)}
	if err := apply.Validate(PromotionDirectionApply); err != nil {
		t.Errorf("apply/apply: %v", err)
	}
	if err := promote.Validate(PromotionDirectionPromote); err != nil {
		t.Errorf("promote/promote: %v", err)
	}
	if err := apply.Validate(PromotionDirectionPromote); err == nil || !strings.Contains(err.Error(), "direction is promote") {
		t.Errorf("apply/promote = %v, want a direction mismatch", err)
	}
	if err := promote.Validate(""); err != nil {
		t.Errorf("promote/any: %v", err)
	}
}

func TestDeletePromotionEnvironment(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := server.client(t).DeletePromotionEnvironment(context.Background(), testEnvironmentID); err != nil {
		t.Fatalf("DeletePromotionEnvironment: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodDelete || req.URL.Path != BasePath+PromotionEnvironmentsPath+"/"+testEnvironmentID {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
}

func TestConnectDisconnectPromotionEnvironment(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionEnvironmentCheckoutBody))
	client := server.client(t)

	checkout, err := client.ConnectPromotionEnvironment(context.Background(), testEnvironmentID)
	if err != nil {
		t.Fatalf("ConnectPromotionEnvironment: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+PromotionEnvironmentsPath+"/"+testEnvironmentID+"/connect" {
		t.Errorf("connect request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("connect body = %q, want empty", body)
	}
	if checkout.EnvironmentID != testEnvironmentID || checkout.Direction != PromotionDirectionApply || !checkout.HasCheckout {
		t.Errorf("checkout = %+v", checkout)
	}

	if _, err := client.DisconnectPromotionEnvironment(context.Background(), testEnvironmentID); err != nil {
		t.Fatalf("DisconnectPromotionEnvironment: %v", err)
	}
	req, _ = server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+PromotionEnvironmentsPath+"/"+testEnvironmentID+"/disconnect" {
		t.Errorf("disconnect request = %s %s", req.Method, req.URL.Path)
	}
}

func TestPromotionEnvironmentProjects(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"projectIds":["p1","p2"]}`))

	projects, err := server.client(t).ListPromotionEnvironmentProjects(context.Background(), testEnvironmentID)
	if err != nil {
		t.Fatalf("ListPromotionEnvironmentProjects: %v", err)
	}
	req, _ := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+PromotionEnvironmentsPath+"/"+testEnvironmentID+"/projects" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if len(projects.ProjectIDs) != 2 {
		t.Errorf("projects = %+v", projects)
	}
}

func TestAddRemovePromotionEnvironmentProject(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"projectId":"p1","environmentId":"`+testEnvironmentID+`"}`))

	link, err := server.client(t).AddProjectToPromotionEnvironment(context.Background(), testEnvironmentID, "p1")
	if err != nil {
		t.Fatalf("AddProjectToPromotionEnvironment: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+PromotionEnvironmentsPath+"/"+testEnvironmentID+"/projects/p1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if link.ProjectID != "p1" || link.EnvironmentID != testEnvironmentID {
		t.Errorf("link = %+v", link)
	}

	removeServer := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if err := removeServer.client(t).RemoveProjectFromPromotionEnvironment(context.Background(), testEnvironmentID, "p1"); err != nil {
		t.Fatalf("RemoveProjectFromPromotionEnvironment: %v", err)
	}
	req, _ = removeServer.last(t)
	if req.Method != http.MethodDelete || req.URL.Path != BasePath+PromotionEnvironmentsPath+"/"+testEnvironmentID+"/projects/p1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestPromotionEnvironmentProjectEscapesID(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"projectId":"a/b c","environmentId":"`+testEnvironmentID+`"}`))

	if _, err := server.client(t).AddProjectToPromotionEnvironment(context.Background(), testEnvironmentID, "a/b c"); err != nil {
		t.Fatalf("AddProjectToPromotionEnvironment: %v", err)
	}
	req, _ := server.last(t)
	if !strings.HasSuffix(req.URL.EscapedPath(), "/projects/a%2Fb%20c") {
		t.Errorf("path = %q, want the project ID escaped", req.URL.EscapedPath())
	}
}
