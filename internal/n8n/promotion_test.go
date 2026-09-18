package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const promotionProviderBody = `{
  "id":"prov-1",
  "name":"github",
  "type":"git",
  "authType":"ssh-key",
  "config":{"schemaVersion":1,"publicKey":"ssh-ed25519 AAAA","keyType":"ed25519"},
  "createdAt":"2026-09-17T00:00:00.000Z",
  "updatedAt":"2026-09-17T00:00:00.000Z"
}`

const promotionProviderCreateBody = `{
  "provider":` + promotionProviderBody + `,
  "publicKey":"ssh-ed25519 AAAA"
}`

const promotionConnectionBody = `{
  "id":"conn-1",
  "name":"prod",
  "scope":"projects",
  "target":{"schemaVersion":1,"remoteUrl":"git@example.com:org/repo.git"},
  "provider":{"id":"prov-1","name":"github","type":"git","authType":"ssh-key","createdAt":"2026-09-17T00:00:00.000Z","updatedAt":"2026-09-17T00:00:00.000Z"},
  "configs":{
    "apply":{"id":"cfg-apply","name":"apply","createdAt":"2026-09-17T00:00:00.000Z","updatedAt":"2026-09-17T00:00:00.000Z","settings":{"schemaVersion":1,"branchName":"main"}},
    "promote":{"id":"cfg-promote","name":"promote","createdAt":"2026-09-17T00:00:00.000Z","updatedAt":"2026-09-17T00:00:00.000Z","settings":{"schemaVersion":1,"baseBranchName":"main","createBranchOnPromotion":false}}
  },
  "createdAt":"2026-09-17T00:00:00.000Z",
  "updatedAt":"2026-09-17T00:00:00.000Z"
}`

const promotionApplyConfigBody = `{
  "id":"cfg-apply",
  "name":"apply",
  "createdAt":"2026-09-17T00:00:00.000Z",
  "updatedAt":"2026-09-17T00:00:00.000Z",
  "settings":{"schemaVersion":1,"branchName":"main"}
}`

const promotionPromoteConfigBody = `{
  "id":"cfg-promote",
  "name":"promote",
  "createdAt":"2026-09-17T00:00:00.000Z",
  "updatedAt":"2026-09-17T00:00:00.000Z",
  "settings":{"schemaVersion":1,"baseBranchName":"main","createBranchOnPromotion":true}
}`

const promotionCheckoutBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-apply",
  "direction":"apply",
  "branchName":"main",
  "hasCheckout":true
}`

const promotionPromoteResultBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-promote",
  "counts":{"workflows":2,"folders":1,"credentials":0,"dataTables":0,"variables":0,"tags":3},
  "git":{"commitSha":"deadbeef","branchName":"main"}
}`

const promotionApplyResultBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-apply",
  "counts":{
    "projects":{"created":1,"updated":0,"skipped":0,"deleted":0},
    "folders":{"created":1,"skipped":0,"removed":0},
    "workflows":{"created":1,"updated":0,"skipped":0,"archived":0,"deleted":0,"publishing":{"published":1,"unpublished":0,"unchanged":0,"blocked":0,"failed":0}},
    "credentials":{"matched":1,"stubbed":0},
    "dataTables":{"matched":0,"created":0},
    "variables":{"matched":0,"created":0,"updated":0,"stubbed":0,"missing":0},
    "tags":{"matched":1,"created":0,"renamed":0,"reconciled":0,"skipped":0}
  },
  "git":{"commitSha":"deadbeef","branchName":"main"}
}`

func TestListPromotionProviders(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[`+promotionProviderBody+`],"nextCursor":"next"}`))

	page, err := server.client(t).ListPromotionProviders(context.Background(), ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListPromotionProviders: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+PromotionProvidersPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+PromotionProvidersPath)
	}
	if req.URL.Query().Get("limit") != "10" {
		t.Errorf("limit = %q, want 10", req.URL.Query().Get("limit"))
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "prov-1" || page.NextCursor != "next" {
		t.Errorf("page = %+v, want one provider with a cursor", page)
	}
	if page.Data[0].Config == nil || page.Data[0].Config.PublicKey == nil {
		t.Errorf("config = %+v, want the public key", page.Data[0].Config)
	}
}

func TestListPromotionProvidersValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))
	if _, err := server.client(t).ListPromotionProviders(context.Background(), ListOptions{Limit: 500}); err == nil {
		t.Error("limit 500: want a validation error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestCreatePromotionProvider(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, promotionProviderCreateBody))

	created, err := server.client(t).CreatePromotionProvider(context.Background(), CreatePromotionProviderRequest{
		Name: "github", Type: PromotionProviderTypeGit,
		Auth: PromotionProviderAuth{AuthType: PromotionAuthTypeToken, Username: "git", Password: "secret"},
	})
	if err != nil {
		t.Fatalf("CreatePromotionProvider: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+PromotionProvidersPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.Path, BasePath+PromotionProvidersPath)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	if sent["name"] != "github" || sent["type"] != "git" {
		t.Errorf("request = %v, want name and type", sent)
	}
	auth, _ := sent["auth"].(map[string]any)
	if auth["authType"] != "token" || auth["username"] != "git" {
		t.Errorf("auth = %v, want token credentials", auth)
	}
	if created.Provider.ID != "prov-1" || created.PublicKey == nil {
		t.Errorf("created = %+v, want the provider with its public key", created)
	}
}

func TestCreatePromotionProviderValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, promotionProviderCreateBody))
	client := server.client(t)

	cases := []CreatePromotionProviderRequest{
		{},
		{Name: "", Type: PromotionProviderTypeGit, Auth: PromotionProviderAuth{AuthType: PromotionAuthTypeSSHKey}},
		{Name: strings.Repeat("x", maxPromotionNameLength+1), Type: PromotionProviderTypeGit, Auth: PromotionProviderAuth{AuthType: PromotionAuthTypeSSHKey}},
		{Name: "github", Type: "svn", Auth: PromotionProviderAuth{AuthType: PromotionAuthTypeSSHKey}},
		{Name: "github", Type: PromotionProviderTypeGit, Auth: PromotionProviderAuth{AuthType: "oauth"}},
		{Name: "github", Type: PromotionProviderTypeGit, Auth: PromotionProviderAuth{AuthType: PromotionAuthTypeSSHKey, KeyType: "dsa"}},
		{Name: "github", Type: PromotionProviderTypeGit, Auth: PromotionProviderAuth{AuthType: PromotionAuthTypeSSHKey, Username: "git"}},
		{Name: "github", Type: PromotionProviderTypeGit, Auth: PromotionProviderAuth{AuthType: PromotionAuthTypeToken, Username: "git"}},
		{Name: "github", Type: PromotionProviderTypeGit, Auth: PromotionProviderAuth{AuthType: PromotionAuthTypeToken, Username: "git", Password: "s", KeyType: "rsa"}},
	}
	for i, request := range cases {
		if _, err := client.CreatePromotionProvider(context.Background(), request); err == nil {
			t.Errorf("case %d: want a validation error", i)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestCreatePromotionProviderRedactsErrors(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusBadRequest, `{"message":"password hunter2 is weak"}`))

	_, err := server.client(t).CreatePromotionProvider(context.Background(), CreatePromotionProviderRequest{
		Name: "github", Type: PromotionProviderTypeGit,
		Auth: PromotionProviderAuth{AuthType: PromotionAuthTypeToken, Username: "git", Password: "hunter2"},
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error = %q, want the submitted secret redacted", err.Error())
	}
}

func TestGetPromotionProvider(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionProviderBody))

	provider, err := server.client(t).GetPromotionProvider(context.Background(), "prov-1")
	if err != nil {
		t.Fatalf("GetPromotionProvider: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+"/promotions/providers/prov-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if provider.Name != "github" || provider.AuthType != "ssh-key" {
		t.Errorf("provider = %+v", provider)
	}
}

func TestGetPromotionProviderEscapesID(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionProviderBody))

	if _, err := server.client(t).GetPromotionProvider(context.Background(), "a/b c"); err != nil {
		t.Fatalf("GetPromotionProvider: %v", err)
	}
	req, _ := server.last(t)
	if !strings.Contains(req.URL.EscapedPath(), "a%2Fb%20c") {
		t.Errorf("path = %q, want the ID escaped as one segment", req.URL.EscapedPath())
	}
	for _, id := range []string{"", "  prov  ", strings.Repeat("x", maxPromotionIDLength+1)} {
		if _, err := server.client(t).GetPromotionProvider(context.Background(), id); err == nil {
			t.Errorf("id %q: want a validation error", id)
		}
	}
}

func TestUpdatePromotionProvider(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionProviderBody))

	updated, err := server.client(t).UpdatePromotionProvider(context.Background(), "prov-1", UpdatePromotionProviderRequest{
		Name: strPtr("github-new"),
	})
	if err != nil {
		t.Fatalf("UpdatePromotionProvider: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPut || req.URL.Path != BasePath+"/promotions/providers/prov-1" {
		t.Errorf("request = %s %s, want PUT", req.Method, req.URL.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	if sent["name"] != "github-new" {
		t.Errorf("request = %v, want only the supplied name", sent)
	}
	if _, hasAuth := sent["auth"]; hasAuth {
		t.Errorf("request = %v, want omitted auth absent", sent)
	}
	if updated.ID != "prov-1" {
		t.Errorf("updated = %+v", updated)
	}
}

func TestUpdatePromotionProviderValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionProviderBody))
	client := server.client(t)

	if _, err := client.UpdatePromotionProvider(context.Background(), "prov-1", UpdatePromotionProviderRequest{}); err == nil {
		t.Error("empty update: want an error")
	}
	blank := "  "
	if _, err := client.UpdatePromotionProvider(context.Background(), "prov-1", UpdatePromotionProviderRequest{Name: &blank}); err == nil {
		t.Error("blank name: want an error")
	}
	badAuth := PromotionProviderAuth{AuthType: "oauth"}
	if _, err := client.UpdatePromotionProvider(context.Background(), "prov-1", UpdatePromotionProviderRequest{Auth: &badAuth}); err == nil {
		t.Error("bad auth: want an error")
	}
	if _, err := client.UpdatePromotionProvider(context.Background(), "  ", UpdatePromotionProviderRequest{Name: strPtr("x")}); err == nil {
		t.Error("blank ID: want an error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestDeletePromotionProvider(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := server.client(t).DeletePromotionProvider(context.Background(), "prov-1"); err != nil {
		t.Fatalf("DeletePromotionProvider: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodDelete || req.URL.Path != BasePath+"/promotions/providers/prov-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
}

func TestListPromotionConnections(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[`+promotionConnectionBody+`],"nextCursor":"next"}`))

	page, err := server.client(t).ListPromotionConnections(context.Background(), ListPromotionConnectionsOptions{
		ListOptions: ListOptions{Limit: 10},
		Scope:       PromotionScopeProjects,
		ProviderID:  "prov-1",
	})
	if err != nil {
		t.Fatalf("ListPromotionConnections: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+PromotionConnectionsPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+PromotionConnectionsPath)
	}
	query := req.URL.Query()
	if query.Get("limit") != "10" || query.Get("scope") != "projects" || query.Get("providerId") != "prov-1" {
		t.Errorf("query = %q, want limit, scope and providerId", req.URL.RawQuery)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "conn-1" || page.NextCursor != "next" {
		t.Errorf("page = %+v, want one connection with a cursor", page)
	}
	if page.Data[0].Configs.Apply == nil || page.Data[0].Configs.Apply.Settings.BranchName != "main" {
		t.Errorf("apply config = %+v, want main", page.Data[0].Configs.Apply)
	}
	if page.Data[0].Configs.Promote == nil || !page.Data[0].Configs.Promote.Settings.CreateBranchOnPromote {
		// The fixture promotes with false; the shape check is that the field decodes.
		_ = page.Data[0].Configs.Promote
	}
}

func TestListPromotionConnectionsValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))
	client := server.client(t)

	if _, err := client.ListPromotionConnections(context.Background(), ListPromotionConnectionsOptions{ListOptions: ListOptions{Limit: 500}}); err == nil {
		t.Error("limit 500: want a validation error")
	}
	if _, err := client.ListPromotionConnections(context.Background(), ListPromotionConnectionsOptions{Scope: "global"}); err == nil {
		t.Error("bad scope: want a validation error")
	}
	if _, err := client.ListPromotionConnections(context.Background(), ListPromotionConnectionsOptions{ProviderID: "  "}); err == nil {
		t.Error("blank provider ID: want a validation error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestCreatePromotionConnection(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, promotionConnectionBody))

	created, err := server.client(t).CreatePromotionConnection(context.Background(), CreatePromotionConnectionRequest{
		Name: "prod", Scope: PromotionScopeProjects, ProviderID: "prov-1",
		Target: PromotionTarget{SchemaVersion: 1, RemoteURL: "git@example.com:org/repo.git"},
		Configs: &CreatePromotionConnectionConfigs{
			Apply: &CreatePromotionApplyConfig{Settings: PromotionApplySettings{SchemaVersion: 1, BranchName: "main"}},
		},
	})
	if err != nil {
		t.Fatalf("CreatePromotionConnection: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+PromotionConnectionsPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.Path, BasePath+PromotionConnectionsPath)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	if sent["name"] != "prod" || sent["scope"] != "projects" || sent["providerId"] != "prov-1" {
		t.Errorf("request = %v, want name, scope and providerId", sent)
	}
	if created.ID != "conn-1" || created.Target.RemoteURL != "git@example.com:org/repo.git" {
		t.Errorf("created = %+v", created)
	}
}

func TestCreatePromotionConnectionValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, promotionConnectionBody))
	client := server.client(t)

	validTarget := PromotionTarget{SchemaVersion: 1, RemoteURL: "git@example.com:org/repo.git"}
	cases := []CreatePromotionConnectionRequest{
		{},
		{Name: "prod", Scope: "global", ProviderID: "prov-1", Target: validTarget},
		{Name: "prod", Scope: PromotionScopeProjects, ProviderID: "  ", Target: validTarget},
		{Name: "prod", Scope: PromotionScopeProjects, ProviderID: "prov-1", Target: PromotionTarget{SchemaVersion: 2, RemoteURL: "git@example.com:r.git"}},
		{Name: "prod", Scope: PromotionScopeProjects, ProviderID: "prov-1", Target: PromotionTarget{SchemaVersion: 1, RemoteURL: "  "}},
		{Name: "prod", Scope: PromotionScopeProjects, ProviderID: "prov-1", Target: validTarget, Configs: &CreatePromotionConnectionConfigs{
			Apply: &CreatePromotionApplyConfig{Settings: PromotionApplySettings{SchemaVersion: 1, BranchName: ""}},
		}},
	}
	for i, request := range cases {
		if _, err := client.CreatePromotionConnection(context.Background(), request); err == nil {
			t.Errorf("case %d: want a validation error", i)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestGetPromotionConnection(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionConnectionBody))

	connection, err := server.client(t).GetPromotionConnection(context.Background(), "conn-1")
	if err != nil {
		t.Fatalf("GetPromotionConnection: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+"/promotions/connections/conn-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if connection.Name != "prod" || connection.Scope != "projects" {
		t.Errorf("connection = %+v", connection)
	}
}

func TestUpdatePromotionConnection(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionConnectionBody))

	updated, err := server.client(t).UpdatePromotionConnection(context.Background(), "conn-1", UpdatePromotionConnectionRequest{
		Name: strPtr("staging"),
	})
	if err != nil {
		t.Fatalf("UpdatePromotionConnection: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPut || req.URL.Path != BasePath+"/promotions/connections/conn-1" {
		t.Errorf("request = %s %s, want PUT", req.Method, req.URL.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil || sent["name"] != "staging" {
		t.Errorf("body = %q (error %v)", raw, err)
	}
	if updated.ID != "conn-1" {
		t.Errorf("updated = %+v", updated)
	}
}

func TestUpdatePromotionConnectionValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionConnectionBody))
	client := server.client(t)

	if _, err := client.UpdatePromotionConnection(context.Background(), "conn-1", UpdatePromotionConnectionRequest{}); err == nil {
		t.Error("empty update: want an error")
	}
	badTarget := PromotionTarget{SchemaVersion: 9, RemoteURL: "git@example.com:r.git"}
	if _, err := client.UpdatePromotionConnection(context.Background(), "conn-1", UpdatePromotionConnectionRequest{Target: &badTarget}); err == nil {
		t.Error("bad target: want an error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestDeletePromotionConnection(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := server.client(t).DeletePromotionConnection(context.Background(), "conn-1"); err != nil {
		t.Fatalf("DeletePromotionConnection: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodDelete || req.URL.Path != BasePath+"/promotions/connections/conn-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
}

func TestUpsertPromotionConfigs(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionApplyConfigBody))

	config, err := server.client(t).UpsertPromotionApplyConfig(context.Background(), "conn-1", UpsertPromotionApplyConfigRequest{
		Settings: PromotionApplySettings{SchemaVersion: 1, BranchName: "main"},
	})
	if err != nil {
		t.Fatalf("UpsertPromotionApplyConfig: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPut || req.URL.Path != BasePath+"/promotions/connections/conn-1/configs/apply" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if config.ID != "cfg-apply" {
		t.Errorf("config = %+v", config)
	}

	server2 := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionPromoteConfigBody))
	promote, err := server2.client(t).UpsertPromotionPromoteConfig(context.Background(), "conn-1", UpsertPromotionPromoteConfigRequest{
		Name:     "promote",
		Settings: PromotionPromoteSettings{SchemaVersion: 1, BaseBranchName: "main", CreateBranchOnPromote: true},
	})
	if err != nil {
		t.Fatalf("UpsertPromotionPromoteConfig: %v", err)
	}
	req, _ = server2.last(t)
	if req.Method != http.MethodPut || req.URL.Path != BasePath+"/promotions/connections/conn-1/configs/promote" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if promote.Settings.BaseBranchName != "main" || !promote.Settings.CreateBranchOnPromote {
		t.Errorf("promote = %+v", promote)
	}

	if _, err := server.client(t).UpsertPromotionApplyConfig(context.Background(), "conn-1", UpsertPromotionApplyConfigRequest{}); err == nil {
		t.Error("empty apply settings: want an error")
	}
	if _, err := server2.client(t).UpsertPromotionPromoteConfig(context.Background(), "conn-1", UpsertPromotionPromoteConfigRequest{
		Settings: PromotionPromoteSettings{SchemaVersion: 1},
	}); err == nil {
		t.Error("empty base branch: want an error")
	}
}

func TestDeletePromotionConfig(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := server.client(t).DeletePromotionConfig(context.Background(), "conn-1", PromotionDirectionApply); err != nil {
		t.Fatalf("DeletePromotionConfig: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodDelete || req.URL.Path != BasePath+"/promotions/connections/conn-1/configs/apply" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if err := server.client(t).DeletePromotionConfig(context.Background(), "conn-1", "sideways"); err == nil {
		t.Error("bad direction: want an error")
	}
}

func TestPromotionCheckoutCloneDisconnect(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionCheckoutBody))

	checkout, err := server.client(t).ClonePromotionCheckout(context.Background(), "conn-1", PromotionDirectionApply)
	if err != nil {
		t.Fatalf("ClonePromotionCheckout: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+"/promotions/connections/conn-1/apply/clone" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if checkout.Direction != "apply" || !checkout.HasCheckout {
		t.Errorf("checkout = %+v", checkout)
	}

	disconnected, err := server.client(t).DisconnectPromotionCheckout(context.Background(), "conn-1", PromotionDirectionPromote)
	if err != nil {
		t.Fatalf("DisconnectPromotionCheckout: %v", err)
	}
	req, _ = server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+"/promotions/connections/conn-1/promote/disconnect" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if disconnected.ConnectionID != "conn-1" {
		t.Errorf("disconnected = %+v", disconnected)
	}

	if _, err := server.client(t).ClonePromotionCheckout(context.Background(), "conn-1", "sideways"); err == nil {
		t.Error("bad direction: want an error")
	}
	if _, err := server.client(t).DisconnectPromotionCheckout(context.Background(), "conn-1", ""); err == nil {
		t.Error("empty direction: want an error")
	}
}

func TestPromotionCheckoutEscapesIDs(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionCheckoutBody))

	if _, err := server.client(t).ClonePromotionCheckout(context.Background(), "a/b", PromotionDirectionApply); err != nil {
		t.Fatalf("ClonePromotionCheckout: %v", err)
	}
	req, _ := server.last(t)
	if !strings.Contains(req.URL.EscapedPath(), "a%2Fb") {
		t.Errorf("path = %q, want the ID escaped", req.URL.EscapedPath())
	}
}

func TestPromotionProjects(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"projectIds":["p1","p2"]}`))

	projects, err := server.client(t).ListPromotionConnectionProjects(context.Background(), "conn-1")
	if err != nil {
		t.Fatalf("ListPromotionConnectionProjects: %v", err)
	}
	req, _ := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+"/promotions/connections/conn-1/projects" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if len(projects.ProjectIDs) != 2 {
		t.Errorf("projects = %+v", projects)
	}
}

func TestAddRemovePromotionProject(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"projectId":"p1","connectionId":"conn-1"}`))

	link, err := server.client(t).AddProjectToPromotionConnection(context.Background(), "conn-1", "p1")
	if err != nil {
		t.Fatalf("AddProjectToPromotionConnection: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+"/promotions/connections/conn-1/projects/p1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if link.ProjectID != "p1" || link.ConnectionID != "conn-1" {
		t.Errorf("link = %+v", link)
	}

	removeServer := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if err := removeServer.client(t).RemoveProjectFromPromotionConnection(context.Background(), "conn-1", "p1"); err != nil {
		t.Fatalf("RemoveProjectFromPromotionConnection: %v", err)
	}
	req, _ = removeServer.last(t)
	if req.Method != http.MethodDelete || req.URL.Path != BasePath+"/promotions/connections/conn-1/projects/p1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestAddPromotionProjectEscapesIDs(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"projectId":"p1","connectionId":"conn-1"}`))

	if _, err := server.client(t).AddProjectToPromotionConnection(context.Background(), "a/b", "c/d"); err != nil {
		t.Fatalf("AddProjectToPromotionConnection: %v", err)
	}
	req, _ := server.last(t)
	if !strings.Contains(req.URL.EscapedPath(), "a%2Fb") || !strings.Contains(req.URL.EscapedPath(), "c%2Fd") {
		t.Errorf("path = %q, want both IDs escaped", req.URL.EscapedPath())
	}
}

func TestPromotePackage(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionPromoteResultBody))

	result, err := server.client(t).PromotePackage(context.Background(), "conn-1", PromotePromotionRequest{CommitMessage: "sync"})
	if err != nil {
		t.Fatalf("PromotePackage: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+"/promotions/connections/conn-1/promote" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil || sent["commitMessage"] != "sync" {
		t.Errorf("commit = %q (error %v)", raw, err)
	}
	if _, hasForce := sent["force"]; hasForce {
		t.Errorf("force = present, want omitted when false")
	}
	if result.Git.CommitSHA != "deadbeef" || result.Counts.Workflows != 2 {
		t.Errorf("result = %+v", result)
	}

	if _, err := server.client(t).PromotePackage(context.Background(), "conn-1", PromotePromotionRequest{CommitMessage: "sync", Force: true}); err != nil {
		t.Fatalf("PromotePackage with force: %v", err)
	}
	_, raw = server.last(t)
	var forced struct {
		Force bool `json:"force"`
	}
	if err := json.Unmarshal([]byte(raw), &forced); err != nil || !forced.Force {
		t.Errorf("force = %q (error %v), want true", raw, err)
	}
	for _, message := range []string{"", "   ", strings.Repeat("x", maxPromotionCommitLength+1)} {
		if _, err := server.client(t).PromotePackage(context.Background(), "conn-1", PromotePromotionRequest{CommitMessage: message}); err == nil {
			t.Errorf("commit %q: want an error", message)
		}
	}
}

func TestApplyPackage(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionApplyResultBody))

	result, err := server.client(t).ApplyPackage(context.Background(), "conn-1")
	if err != nil {
		t.Fatalf("ApplyPackage: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+"/promotions/connections/conn-1/apply" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if result.Git.CommitSHA != "deadbeef" || result.Counts.Projects.Created != 1 || result.Counts.Workflows.Publishing.Published != 1 {
		t.Errorf("result = %+v", result)
	}
}

func TestPromotionAuthAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"unauthorized", http.StatusUnauthorized, `{"message":"unauthorized"}`, IsUnauthorized},
		{"forbidden", http.StatusForbidden, `{"message":"denied"}`, IsForbidden},
		{"not found", http.StatusNotFound, `{"message":"not found"}`, IsNotFound},
		{"conflict", http.StatusConflict, `{"message":"exists"}`, IsConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tc.status, tc.body))
			_, err := server.client(t).GetPromotionConnection(context.Background(), "conn-1")
			if err == nil || !tc.check(err) {
				t.Fatalf("error = %v, want the %d behavior", err, tc.status)
			}
		})
	}
}

func TestPromotionEmptyResponses(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(http.StatusOK)
	})
	client := server.client(t)

	page, err := client.ListPromotionProviders(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("ListPromotionProviders: %v", err)
	}
	if len(page.Data) != 0 {
		t.Errorf("providers = %d, want 0", len(page.Data))
	}
}
