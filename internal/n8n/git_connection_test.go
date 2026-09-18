package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const gitConnectionBody = `{
  "id":"conn-1",
  "name":"prod",
  "repositoryUrl":"git@example.com:org/repo.git",
  "branchName":"main",
  "connectionType":"ssh",
  "publicKey":"ssh-ed25519 AAAAEBBY",
  "keyGeneratorType":"ed25519",
  "baseCommit":"abc123",
  "createdAt":"2026-09-17T00:00:00.000Z",
  "updatedAt":"2026-09-17T00:00:00.000Z"
}`

const gitPushResultBody = `{
  "connectionId":"conn-1",
  "counts":{"workflows":2,"folders":1,"credentials":0,"dataTables":0,"variables":0,"tags":3},
  "commitSha":"deadbeef"
}`

const gitPullResultBody = `{
  "connectionId":"conn-1",
  "counts":{
    "projects":{"created":1,"updated":0,"skipped":0,"deleted":0},
    "folders":{"created":1,"skipped":0,"removed":0},
    "workflows":{"created":1,"updated":0,"skipped":0,"archived":0,"deleted":0,"publishing":{"published":1,"unpublished":0,"unchanged":0,"blocked":0,"failed":0}},
    "credentials":{"matched":1,"stubbed":0},
    "dataTables":{"matched":0,"created":0},
    "variables":{"matched":0,"created":0,"updated":0,"stubbed":0,"missing":0},
    "tags":{"matched":1,"created":0,"renamed":0,"reconciled":0,"skipped":0}
  },
  "commitSha":"deadbeef"
}`

func TestListGitConnections(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[`+gitConnectionBody+`],"nextCursor":"next"}`))

	page, err := server.client(t).ListGitConnections(context.Background(), ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListGitConnections: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+GitConnectionsPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+GitConnectionsPath)
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
	if len(page.Data) != 1 || page.Data[0].ID != "conn-1" || page.NextCursor != "next" {
		t.Errorf("page = %+v, want one connection with a cursor", page)
	}
	if page.Data[0].BranchName == nil || *page.Data[0].BranchName != "main" {
		t.Errorf("branch = %+v, want main", page.Data[0].BranchName)
	}
}

func TestListGitConnectionsPaginationValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))
	if _, err := server.client(t).ListGitConnections(context.Background(), ListOptions{Limit: 500}); err == nil {
		t.Error("limit 500: want a validation error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestCreateGitConnection(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, gitConnectionBody))

	created, err := server.client(t).CreateGitConnection(context.Background(), CreateGitConnectionRequest{
		Name: "prod", RepositoryURL: "git@example.com:org/repo.git", ConnectionType: GitConnectionTypeSSH,
		BranchName: "main", KeyGeneratorType: GitKeyGeneratorED25519, Username: "git", Password: "secret",
	})
	if err != nil {
		t.Fatalf("CreateGitConnection: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+GitConnectionsPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.Path, BasePath+GitConnectionsPath)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	if sent["name"] != "prod" || sent["repositoryUrl"] != "git@example.com:org/repo.git" || sent["connectionType"] != "ssh" {
		t.Errorf("request = %v, want name, repositoryUrl and connectionType", sent)
	}
	if created.ID != "conn-1" || created.PublicKey == nil {
		t.Errorf("created = %+v, want the connection with its public key", created)
	}
}

func TestCreateGitConnectionValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, gitConnectionBody))
	client := server.client(t)

	cases := []CreateGitConnectionRequest{
		{},
		{Name: "", RepositoryURL: "git@example.com:r.git", ConnectionType: GitConnectionTypeSSH},
		{Name: strings.Repeat("x", maxGitConnectionNameLength+1), RepositoryURL: "git@example.com:r.git", ConnectionType: GitConnectionTypeSSH},
		{Name: "prod", RepositoryURL: "   ", ConnectionType: GitConnectionTypeSSH},
		{Name: "prod", RepositoryURL: "git@example.com:r.git", ConnectionType: "svn"},
		{Name: "prod", RepositoryURL: "git@example.com:r.git", ConnectionType: GitConnectionTypeSSH, BranchName: "  "},
		{Name: "prod", RepositoryURL: "git@example.com:r.git", ConnectionType: GitConnectionTypeSSH, KeyGeneratorType: "dsa"},
	}
	for i, request := range cases {
		if _, err := client.CreateGitConnection(context.Background(), request); err == nil {
			t.Errorf("case %d: want a validation error", i)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestCreateGitConnectionRedactsErrors(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusBadRequest, `{"message":"password hunter2 is weak"}`))

	_, err := server.client(t).CreateGitConnection(context.Background(), CreateGitConnectionRequest{
		Name: "prod", RepositoryURL: "git@example.com:r.git", ConnectionType: GitConnectionTypeSSH, Password: "hunter2",
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error = %q, want the submitted secret redacted", err.Error())
	}
}

func TestGetGitConnection(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, gitConnectionBody))

	connection, err := server.client(t).GetGitConnection(context.Background(), "conn-1")
	if err != nil {
		t.Fatalf("GetGitConnection: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+"/git-connections/conn-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if connection.Name != "prod" || connection.RepositoryURL != "git@example.com:org/repo.git" {
		t.Errorf("connection = %+v", connection)
	}
}

func TestGetGitConnectionEscapesID(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, gitConnectionBody))

	if _, err := server.client(t).GetGitConnection(context.Background(), "a/b c"); err != nil {
		t.Fatalf("GetGitConnection: %v", err)
	}
	req, _ := server.last(t)
	if !strings.Contains(req.URL.EscapedPath(), "a%2Fb%20c") {
		t.Errorf("path = %q, want the ID escaped as one segment", req.URL.EscapedPath())
	}

	for _, id := range []string{"", "  conn  "} {
		if _, err := server.client(t).GetGitConnection(context.Background(), id); err == nil {
			t.Errorf("id %q: want a validation error", id)
		}
	}
}

func TestUpdateGitConnection(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, gitConnectionBody))

	updated, err := server.client(t).UpdateGitConnection(context.Background(), "conn-1", UpdateGitConnectionRequest{
		Name: strPtr("staging"), BranchName: strPtr("release"),
	})
	if err != nil {
		t.Fatalf("UpdateGitConnection: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPut || req.URL.Path != BasePath+"/git-connections/conn-1" {
		t.Errorf("request = %s %s, want PUT", req.Method, req.URL.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	if sent["name"] != "staging" || sent["branchName"] != "release" {
		t.Errorf("request = %v, want only the supplied fields", sent)
	}
	if _, hasRepo := sent["repositoryUrl"]; hasRepo {
		t.Errorf("request = %v, want omitted fields absent", sent)
	}
	if updated.ID != "conn-1" {
		t.Errorf("updated = %+v", updated)
	}
}

func TestUpdateGitConnectionValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, gitConnectionBody))
	client := server.client(t)

	if _, err := client.UpdateGitConnection(context.Background(), "conn-1", UpdateGitConnectionRequest{}); err == nil {
		t.Error("empty update: want an error")
	}
	badType := "svn"
	if _, err := client.UpdateGitConnection(context.Background(), "conn-1", UpdateGitConnectionRequest{ConnectionType: &badType}); err == nil {
		t.Error("unknown connection type: want an error")
	}
	blank := "  "
	if _, err := client.UpdateGitConnection(context.Background(), "conn-1", UpdateGitConnectionRequest{Name: &blank}); err == nil {
		t.Error("blank name: want an error")
	}
	if _, err := client.UpdateGitConnection(context.Background(), "  ", UpdateGitConnectionRequest{Name: strPtr("x")}); err == nil {
		t.Error("blank ID: want an error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestDeleteGitConnection(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := server.client(t).DeleteGitConnection(context.Background(), "conn-1"); err != nil {
		t.Fatalf("DeleteGitConnection: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodDelete || req.URL.Path != BasePath+"/git-connections/conn-1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
}

func TestCloneGitConnection(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, gitConnectionBody))

	cloned, err := server.client(t).CloneGitConnection(context.Background(), "conn-1", CloneGitConnectionRequest{BranchName: "release"})
	if err != nil {
		t.Fatalf("CloneGitConnection: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+"/git-connections/conn-1/clone" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil || sent["branchName"] != "release" {
		t.Errorf("branch = %q (error %v), want release", raw, err)
	}
	if cloned.ID != "conn-1" {
		t.Errorf("cloned = %+v", cloned)
	}
}

func TestCloneGitConnectionOmitsEmptyBranch(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, gitConnectionBody))

	if _, err := server.client(t).CloneGitConnection(context.Background(), "conn-1", CloneGitConnectionRequest{}); err != nil {
		t.Fatalf("CloneGitConnection: %v", err)
	}
	_, raw := server.last(t)
	if raw != "" {
		t.Errorf("body = %q, want no body for the configured branch", raw)
	}

	if _, err := server.client(t).CloneGitConnection(context.Background(), "conn-1", CloneGitConnectionRequest{BranchName: "  "}); err == nil {
		t.Error("blank branch: want an error")
	}
}

func TestDisconnectGitConnection(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, gitConnectionBody))

	disconnected, err := server.client(t).DisconnectGitConnection(context.Background(), "conn-1")
	if err != nil {
		t.Fatalf("DisconnectGitConnection: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+"/git-connections/conn-1/disconnect" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if disconnected.ID != "conn-1" {
		t.Errorf("disconnected = %+v", disconnected)
	}
}

func TestGitConnectionProjects(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"projectIds":["p1","p2"]}`))

	projects, err := server.client(t).ListGitConnectionProjects(context.Background(), "conn-1")
	if err != nil {
		t.Fatalf("ListGitConnectionProjects: %v", err)
	}
	req, _ := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+"/git-connections/conn-1/projects" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if len(projects.ProjectIDs) != 2 || projects.ProjectIDs[0] != "p1" {
		t.Errorf("projects = %+v", projects)
	}
}

func TestAddProjectToGitConnection(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"projectId":"p1","gitConnectionId":"conn-1"}`))

	link, err := server.client(t).AddProjectToGitConnection(context.Background(), "conn-1", "p1")
	if err != nil {
		t.Fatalf("AddProjectToGitConnection: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+"/git-connections/conn-1/projects/p1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if link.ProjectID != "p1" || link.GitConnectionID != "conn-1" {
		t.Errorf("link = %+v", link)
	}
}

func TestAddProjectEscapesIDs(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"projectId":"p1","gitConnectionId":"conn-1"}`))

	if _, err := server.client(t).AddProjectToGitConnection(context.Background(), "a/b", "c/d"); err != nil {
		t.Fatalf("AddProjectToGitConnection: %v", err)
	}
	req, _ := server.last(t)
	if !strings.Contains(req.URL.EscapedPath(), "a%2Fb") || !strings.Contains(req.URL.EscapedPath(), "c%2Fd") {
		t.Errorf("path = %q, want both IDs escaped as segments", req.URL.EscapedPath())
	}
}

func TestRemoveProjectFromGitConnection(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := server.client(t).RemoveProjectFromGitConnection(context.Background(), "conn-1", "p1"); err != nil {
		t.Fatalf("RemoveProjectFromGitConnection: %v", err)
	}
	req, _ := server.last(t)
	if req.Method != http.MethodDelete || req.URL.Path != BasePath+"/git-connections/conn-1/projects/p1" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
}

func TestPushGitConnectionProjects(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, gitPushResultBody))

	result, err := server.client(t).PushGitConnectionProjects(context.Background(), "conn-1", PushGitConnectionRequest{CommitMessage: "sync"})
	if err != nil {
		t.Fatalf("PushGitConnectionProjects: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+"/git-connections/conn-1/push" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil || sent["commitMessage"] != "sync" {
		t.Errorf("commit = %q (error %v)", raw, err)
	}
	if _, hasForce := sent["force"]; hasForce {
		t.Errorf("force = present, want omitted when false")
	}
	if result.CommitSHA != "deadbeef" || result.Counts.Workflows != 2 || result.Counts.Tags != 3 {
		t.Errorf("result = %+v", result)
	}
}

func TestPushGitConnectionForceAndValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, gitPushResultBody))

	if _, err := server.client(t).PushGitConnectionProjects(context.Background(), "conn-1", PushGitConnectionRequest{CommitMessage: "sync", Force: true}); err != nil {
		t.Fatalf("PushGitConnectionProjects: %v", err)
	}
	_, raw := server.last(t)
	var sent struct {
		Force bool `json:"force"`
	}
	if err := json.Unmarshal([]byte(raw), &sent); err != nil || !sent.Force {
		t.Errorf("force = %q (error %v), want true", raw, err)
	}

	for _, message := range []string{"", "   ", strings.Repeat("x", maxGitPushCommitLength+1)} {
		if _, err := server.client(t).PushGitConnectionProjects(context.Background(), "conn-1", PushGitConnectionRequest{CommitMessage: message}); err == nil {
			t.Errorf("commit %q: want an error", message)
		}
	}
}

func TestPullGitConnectionProjects(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, gitPullResultBody))

	result, err := server.client(t).PullGitConnectionProjects(context.Background(), "conn-1")
	if err != nil {
		t.Fatalf("PullGitConnectionProjects: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+"/git-connections/conn-1/pull" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if result.CommitSHA != "deadbeef" || result.Counts.Projects.Created != 1 || result.Counts.Workflows.Publishing.Published != 1 {
		t.Errorf("result = %+v", result)
	}
}

func TestGitConnectionAuthAndErrors(t *testing.T) {
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
			_, err := server.client(t).GetGitConnection(context.Background(), "conn-1")
			if err == nil || !tc.check(err) {
				t.Fatalf("error = %v, want the %d behavior", err, tc.status)
			}
		})
	}
}

func TestGitConnectionEmptyResponses(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(http.StatusOK)
	})
	client := server.client(t)

	page, err := client.ListGitConnections(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("ListGitConnections: %v", err)
	}
	if len(page.Data) != 0 {
		t.Errorf("connections = %d, want 0", len(page.Data))
	}
}

func strPtr(value string) *string { return &value }
