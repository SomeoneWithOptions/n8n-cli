package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const sourceControlledFileResponse = `{
  "file":"workflows/abc.json",
  "id":"abc",
  "name":"Example workflow",
  "type":"workflow",
  "status":"modified",
  "location":"local",
  "conflict":false,
  "updatedAt":"2026-09-17T00:00:00.000Z",
  "pushed":false,
  "isLocalPublished":true,
  "isRemoteArchived":false,
  "folderPath":["parent"],
  "remoteFolderPath":["parent"],
  "owner":{"type":"team","projectId":"p1","projectName":"Team"},
  "extraFieldAddedLater":"ignored"
}`

func TestGetSourceControlStatus(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[`+sourceControlledFileResponse+`]}`))

	status, err := server.client(t).GetSourceControlStatus(context.Background(), SourceControlDirectionPush)
	if err != nil {
		t.Fatalf("GetSourceControlStatus: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+SourceControlStatusPath {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, BasePath+SourceControlStatusPath)
	}
	if req.URL.Query().Get("direction") != "push" {
		t.Errorf("direction = %q, want push", req.URL.Query().Get("direction"))
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if len(status.Data) != 1 {
		t.Fatalf("files = %d, want 1", len(status.Data))
	}
	file := status.Data[0]
	if file.ID != "abc" || file.Type != SourceControlledFileWorkflow || file.Status != "modified" || file.Conflict {
		t.Errorf("file = %+v, want the example workflow without conflict", file)
	}
	if file.Pushed == nil || *file.Pushed {
		t.Errorf("pushed = %+v, want explicit false", file.Pushed)
	}
	if file.Owner == nil || file.Owner.ProjectID != "p1" || file.Owner.ProjectName != "Team" {
		t.Errorf("owner = %+v, want the team project", file.Owner)
	}
	if len(file.FolderPath) != 1 || file.FolderPath[0] != "parent" {
		t.Errorf("folderPath = %v, want [parent]", file.FolderPath)
	}
}

func TestGetSourceControlStatusPullDirection(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[]}`))

	status, err := server.client(t).GetSourceControlStatus(context.Background(), SourceControlDirectionPull)
	if err != nil {
		t.Fatalf("GetSourceControlStatus: %v", err)
	}
	if req, _ := server.last(t); req.URL.Query().Get("direction") != "pull" {
		t.Errorf("direction = %q, want pull", req.URL.Query().Get("direction"))
	}
	if len(status.Data) != 0 {
		t.Errorf("files = %d, want 0", len(status.Data))
	}
}

func TestGetSourceControlStatusRejectsBadDirection(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[]}`))
	client := server.client(t)

	for _, direction := range []SourceControlDirection{"", "both", "PUSH"} {
		if _, err := client.GetSourceControlStatus(context.Background(), direction); err == nil {
			t.Errorf("direction %q: want an error", direction)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestPushSourceControl(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[`+sourceControlledFileResponse+`]}`))

	result, err := server.client(t).PushSourceControl(context.Background(), PushSourceControlRequest{
		CommitMessage: "sync workflows",
		FileNames: []SourceControlFileSelector{
			{ID: "abc", Type: SourceControlledFileWorkflow},
			{ID: "cred-1", Type: SourceControlledFileCredential},
		},
	})
	if err != nil {
		t.Fatalf("PushSourceControl: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+SourceControlPushPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.Path, BasePath+SourceControlPushPath)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	if sent["commitMessage"] != "sync workflows" {
		t.Errorf("commitMessage = %v", sent["commitMessage"])
	}
	files, ok := sent["fileNames"].([]any)
	if !ok || len(files) != 2 {
		t.Fatalf("fileNames = %v, want 2 entries", sent["fileNames"])
	}
	if _, hasForce := sent["force"]; hasForce {
		t.Errorf("force = present, want omitted when false")
	}
	if len(result.Data) != 1 || result.Data[0].ID != "abc" {
		t.Errorf("result = %+v, want the pushed file", result)
	}
}

func TestPushSourceControlForceBody(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[]}`))

	_, err := server.client(t).PushSourceControl(context.Background(), PushSourceControlRequest{
		CommitMessage: "force push",
		FileNames:     []SourceControlFileSelector{{ID: "abc", Type: SourceControlledFileWorkflow}},
		Force:         true,
	})
	if err != nil {
		t.Fatalf("PushSourceControl: %v", err)
	}
	_, raw := server.last(t)
	var sent struct {
		Force bool `json:"force"`
	}
	if err := json.Unmarshal([]byte(raw), &sent); err != nil || !sent.Force {
		t.Errorf("force = %q (error %v), want true", raw, err)
	}
}

func TestPushSourceControlValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[]}`))
	client := server.client(t)

	cases := []PushSourceControlRequest{
		{},
		{CommitMessage: "   ", FileNames: []SourceControlFileSelector{{ID: "a", Type: SourceControlledFileWorkflow}}},
		{CommitMessage: strings.Repeat("x", maxSourceControlCommitMessage+1), FileNames: []SourceControlFileSelector{{ID: "a", Type: SourceControlledFileWorkflow}}},
		{CommitMessage: "msg"},
		{CommitMessage: "msg", FileNames: []SourceControlFileSelector{{ID: "", Type: SourceControlledFileWorkflow}}},
		{CommitMessage: "msg", FileNames: []SourceControlFileSelector{{ID: "a", Type: "unknown"}}},
	}
	for i, request := range cases {
		if _, err := client.PushSourceControl(context.Background(), request); err == nil {
			t.Errorf("case %d: want a validation error", i)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestPushSourceControlConflict(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusConflict, `{"message":"unresolved conflicts","conflicts":[`+sourceControlledFileResponse+`]}`))

	_, err := server.client(t).PushSourceControl(context.Background(), PushSourceControlRequest{
		CommitMessage: "msg",
		FileNames:     []SourceControlFileSelector{{ID: "abc", Type: SourceControlledFileWorkflow}},
	})
	if err == nil || !IsConflict(err) || !strings.Contains(err.Error(), "unresolved conflicts") {
		t.Fatalf("error = %v, want the 409 message", err)
	}
}

func TestPullSourceControl(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `[`+sourceControlledFileResponse+`]`))

	files, err := server.client(t).PullSourceControl(context.Background(), PullSourceControlRequest{
		AutoPublish: SourceControlAutoPublishPublished,
	})
	if err != nil {
		t.Fatalf("PullSourceControl: %v", err)
	}
	req, raw := server.last(t)
	if req.Method != http.MethodPost || req.URL.Path != BasePath+SourceControlPullPath {
		t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.Path, BasePath+SourceControlPullPath)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	if sent["autoPublish"] != "published" {
		t.Errorf("autoPublish = %v, want published", sent["autoPublish"])
	}
	if len(files) != 1 || files[0].ID != "abc" {
		t.Errorf("files = %+v, want the pulled file", files)
	}
}

func TestPullSourceControlDefaultsToNone(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `[]`))

	files, err := server.client(t).PullSourceControl(context.Background(), PullSourceControlRequest{})
	if err != nil {
		t.Fatalf("PullSourceControl: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("files = %d, want 0", len(files))
	}
	_, raw := server.last(t)
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	if sent["autoPublish"] != "none" {
		t.Errorf("autoPublish = %v, want the documented none default", sent["autoPublish"])
	}
}

func TestPullSourceControlValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `[]`))
	client := server.client(t)

	if _, err := client.PullSourceControl(context.Background(), PullSourceControlRequest{AutoPublish: "sometimes"}); err == nil {
		t.Error("unknown auto-publish: want an error")
	}
	if len(server.requests) != 0 {
		t.Errorf("requests = %d, want 0 before transport", len(server.requests))
	}
}

func TestPullSourceControlConflict(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusConflict, `[`+sourceControlledFileResponse+`]`))

	_, err := server.client(t).PullSourceControl(context.Background(), PullSourceControlRequest{})
	if err == nil || !IsConflict(err) {
		t.Fatalf("error = %v, want a 409", err)
	}
}

func TestSourceControlDecoding(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[`+sourceControlledFileResponse+`]}`))

	status, err := server.client(t).GetSourceControlStatus(context.Background(), SourceControlDirectionPush)
	if err != nil {
		t.Fatalf("GetSourceControlStatus: %v", err)
	}
	want := SourceControlledFile{
		File:             "workflows/abc.json",
		ID:               "abc",
		Name:             "Example workflow",
		Type:             SourceControlledFileWorkflow,
		Status:           "modified",
		Location:         "local",
		UpdatedAt:        "2026-09-17T00:00:00.000Z",
		FolderPath:       []string{"parent"},
		RemoteFolderPath: []string{"parent"},
		Owner:            &SourceControlOwner{Type: "team", ProjectID: "p1", ProjectName: "Team"},
	}
	got := status.Data[0]
	// Optional booleans and raw diagnostics are checked separately because
	// cmp does not compare them usefully in one diff.
	got.Pushed, got.IsLocalPublished, got.IsRemoteArchived = nil, nil, nil
	got.PublishingErrorDetails, got.ContentImportPolicy = nil, nil
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("file mismatch (-want +got):\n%s", diff)
	}
}

func TestSourceControlAuthAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"unauthorized", http.StatusUnauthorized, `{"message":"unauthorized"}`, IsUnauthorized},
		{"forbidden", http.StatusForbidden, `{"message":"denied"}`, IsForbidden},
		{"not found", http.StatusNotFound, `{"message":"not found"}`, IsNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tc.status, tc.body))
			_, err := server.client(t).GetSourceControlStatus(context.Background(), SourceControlDirectionPush)
			if err == nil || !tc.check(err) {
				t.Fatalf("error = %v, want the %d behavior", err, tc.status)
			}
		})
	}
}

func TestSourceControlEmptyResponses(t *testing.T) {
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(http.StatusOK)
	})
	client := server.client(t)

	status, err := client.GetSourceControlStatus(context.Background(), SourceControlDirectionPush)
	if err != nil {
		t.Fatalf("GetSourceControlStatus: %v", err)
	}
	if len(status.Data) != 0 {
		t.Errorf("status files = %d, want 0", len(status.Data))
	}
}
