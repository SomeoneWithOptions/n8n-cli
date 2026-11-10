package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// n8n 2.47 apply shapes: blocked carries check instead of preflight, with
// workflow, data-table, tag, project, and moved conflicts; preview-changed
// joins the status union; project apply bodies take expectedPreviewRevision;
// and environments list their missing projects.

const previewRevision = "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"

const checkBlockedResultBody = `{
  "environmentId":"` + testEnvironmentID + `",
  "git":{"commitSha":"` + selectionSHA + `","branchName":"main"},
  "status":"blocked",
  "check":{
    "missingProjects":[{"id":"p-9","name":"Ops"}],
    "missingBindings":[],
    "accessRequirements":[],
    "conflicts":[
      {"kind":"workflow","code":"lineage-conflict","reason":"history diverged","consumers":[{"project":{"id":"p1","name":"Sales"},"workflows":[{"id":"w1","name":"Lead"}]}]},
      {"kind":"tag","code":"tag-conflict","id":"t1","name":"prod","reason":"renamed here","consumers":[{"project":{"id":"p1","name":"Sales"},"workflows":[{"id":"w1","name":"Lead"}]}]},
      {"kind":"project","code":"project-not-on-branch","project":{"id":"p2","name":"Ops"},"reason":"absent"},
      {"kind":"folder","code":"moved","id":"f1","name":"Inbox","project":{"id":"p1","name":"Sales"},"projectExists":false,"instanceProject":{"id":"p3","name":"Support"}}
    ],
    "warnings":[]
  }
}`

func TestApplyPromotionResultDecodesCheck(t *testing.T) {
	var result ApplyPromotionResult
	if err := json.Unmarshal([]byte(checkBlockedResultBody), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != PromotionApplyStatusBlocked || result.Applied() || result.Preflight != nil || result.Extra != nil {
		t.Fatalf("result = %+v, want blocked with check only", result)
	}
	check := result.Blockers()
	if check == nil || check != result.Check || len(check.MissingProjects) != 1 || len(check.Conflicts) != 4 {
		t.Fatalf("blockers = %+v, want the check", check)
	}
	workflow, tag, project, moved := check.Conflicts[0], check.Conflicts[1], check.Conflicts[2], check.Conflicts[3]
	if workflow.Reason != "history diverged" || len(workflow.Consumers) != 1 {
		t.Errorf("workflow conflict = %+v", workflow)
	}
	if tag.ID != "t1" || tag.Reason != "renamed here" {
		t.Errorf("tag conflict = %+v", tag)
	}
	if project.Project == nil || project.Project.ID != "p2" || project.Reason != "absent" {
		t.Errorf("project conflict = %+v", project)
	}
	if moved.Code != PromotionConflictMoved || moved.ProjectExists == nil || *moved.ProjectExists ||
		moved.InstanceProject == nil || moved.InstanceProject.ID != "p3" || moved.Project.ID != "p1" {
		t.Errorf("moved conflict = %+v", moved)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(checkBlockedResultBody), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("response not preserved: %s", encoded)
	}

	// Older instances still send preflight; Blockers falls back to it.
	var older ApplyPromotionResult
	if err := json.Unmarshal([]byte(envBlockedResultBody), &older); err != nil {
		t.Fatal(err)
	}
	if older.Check != nil || older.Blockers() == nil || older.Blockers() != older.Preflight {
		t.Errorf("older blockers = %+v, want preflight", older.Blockers())
	}
	var none ApplyPromotionResult
	if none.Blockers() != nil {
		t.Error("zero result has blockers")
	}
}

func TestApplyPromotionResultPreviewChanged(t *testing.T) {
	body := `{"environmentId":"` + testEnvironmentID + `","git":{"commitSha":"` + selectionSHA + `","branchName":"main"},"status":"preview-changed"}`
	var result ApplyPromotionResult
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != PromotionApplyStatusPreviewChanged || result.Applied() || result.Blockers() != nil {
		t.Errorf("result = %+v, want preview-changed that wrote nothing", result)
	}
}

func TestProjectSelectionExpectedPreviewRevision(t *testing.T) {
	source := PromotionExpectedSource{EnvironmentID: testEnvironmentID, BranchName: "main", CommitSHA: selectionSHA}
	cases := []struct {
		name, path, body string
		call             func(context.Context, *Client) (any, error)
	}{
		{"apply omits an empty revision", "/promotions/projects/p1/apply", `{"workflowIds":["w1"]}`, func(ctx context.Context, c *Client) (any, error) {
			return c.ApplyProjectSelection(ctx, "p1", ApplyProjectSelectionRequest{WorkflowIDs: []string{"w1"}})
		}},
		{"apply sends the revision", "/promotions/projects/p1/apply", `{"workflowIds":["w1"],"expectedPreviewRevision":"` + previewRevision + `"}`, func(ctx context.Context, c *Client) (any, error) {
			return c.ApplyProjectSelection(ctx, "p1", ApplyProjectSelectionRequest{WorkflowIDs: []string{"w1"}, ExpectedPreviewRevision: previewRevision})
		}},
		{"continue sends the revision", "/promotions/projects/p1/apply/continue", `{"workflowIds":["w1"],"expectedSource":` + envSourceJSON + `,"expectedPreviewRevision":"` + previewRevision + `"}`, func(ctx context.Context, c *Client) (any, error) {
			return c.ContinueApplyProjectSelection(ctx, "p1", ContinueApplyProjectSelectionRequest{WorkflowIDs: []string{"w1"}, ExpectedSource: source, ExpectedPreviewRevision: previewRevision})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, envAppliedResultBody))
			if _, err := tc.call(context.Background(), server.client(t)); err != nil {
				t.Fatal(err)
			}
			req, body := server.last(t)
			if req.URL.EscapedPath() != BasePath+tc.path || body != tc.body {
				t.Errorf("request = %s %s, want %s %s", req.URL.EscapedPath(), body, BasePath+tc.path, tc.body)
			}
		})
	}

	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, envAppliedResultBody))
	client := server.client(t)
	for _, bad := range []string{"abc", strings.ToUpper(previewRevision), previewRevision + "0"} {
		_, err := client.ApplyProjectSelection(context.Background(), "p1", ApplyProjectSelectionRequest{WorkflowIDs: []string{"w1"}, ExpectedPreviewRevision: bad})
		if err == nil || !strings.Contains(err.Error(), "64 lowercase hex") {
			t.Errorf("apply revision %q: error = %v", bad, err)
		}
		_, err = client.ContinueApplyProjectSelection(context.Background(), "p1", ContinueApplyProjectSelectionRequest{WorkflowIDs: []string{"w1"}, ExpectedSource: source, ExpectedPreviewRevision: bad})
		if err == nil || !strings.Contains(err.Error(), "64 lowercase hex") {
			t.Errorf("continue revision %q: error = %v", bad, err)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("%d request(s) sent for invalid revisions", len(server.requests))
	}
}

const missingProjectsBody = `{
  "environmentId":"` + testEnvironmentID + `",
  "git":{"commitSha":"` + selectionSHA + `","branchName":"main"},
  "data":[{"id":"p-9","name":"Ops","icon":{"type":"emoji","value":"📣"},"description":"Operations","customTelemetryTags":[{"key":"team","value":"ops"}],"future":true}]
}`

func TestGetPromotionEnvironmentMissingProjects(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, missingProjectsBody))
	result, err := server.client(t).GetPromotionEnvironmentMissingProjects(context.Background(), testEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := server.last(t)
	if want := BasePath + PromotionEnvironmentsPath + "/" + testEnvironmentID + "/missing-projects"; req.Method != http.MethodGet || req.URL.EscapedPath() != want {
		t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.EscapedPath(), want)
	}
	if result.EnvironmentID != testEnvironmentID || result.Git == nil || result.Git.CommitSHA != selectionSHA ||
		len(result.Data) != 1 || result.Data[0].ID != "p-9" || result.Data[0].Description != "Operations" {
		t.Errorf("result = %+v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(missingProjectsBody), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("response not preserved: %s", encoded)
	}

	empty := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"environmentId":"`+testEnvironmentID+`","git":null,"data":[]}`))
	result, err = empty.client(t).GetPromotionEnvironmentMissingProjects(context.Background(), testEnvironmentID)
	if err != nil || result.Git != nil || len(result.Data) != 0 {
		t.Errorf("empty result = %+v, %v", result, err)
	}

	for _, status := range []int{400, 403, 404, 503} {
		s := newCommunityPackageServer(t, packageJSONHandler(status, `{"message":"failure"}`))
		if _, err := s.client(t).GetPromotionEnvironmentMissingProjects(context.Background(), testEnvironmentID); !IsStatus(err, status) {
			t.Errorf("status %d: error = %v", status, err)
		}
	}

	invalid := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, missingProjectsBody))
	if _, err := invalid.client(t).GetPromotionEnvironmentMissingProjects(context.Background(), " "); err == nil {
		t.Error("blank environment ID accepted")
	}
	if len(invalid.requests) != 0 {
		t.Error("invalid ID reached the instance")
	}
}
