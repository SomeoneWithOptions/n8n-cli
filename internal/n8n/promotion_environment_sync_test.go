package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// n8n 2.46 apply, continue, and promote shapes: environmentId replaces
// connectionId and configId, folders gain updated/moved/renamed, workflows
// gain moved, credentials gain updated, and promote can add review and
// warnings.

const envSourceJSON = `{"environmentId":"` + testEnvironmentID + `","branchName":"main","commitSha":"` + selectionSHA + `"}`

const envAppliedResultBody = `{
  "environmentId":"` + testEnvironmentID + `",
  "git":{"commitSha":"` + selectionSHA + `","branchName":"main"},
  "status":"applied",
  "counts":{
    "projects":{"created":0,"updated":1,"skipped":0,"deleted":0},
    "folders":{"created":1,"updated":3,"skipped":0,"removed":0,"moved":2,"renamed":1},
    "workflows":{"created":0,"updated":4,"skipped":0,"archived":0,"deleted":0,"moved":2,"publishing":{"published":0,"unpublished":0,"unchanged":4,"blocked":0,"failed":0}},
    "credentials":{"matched":1,"stubbed":0,"updated":5},
    "dataTables":{"matched":0,"created":0,"updated":0},
    "variables":{"matched":0,"created":0,"updated":0,"stubbed":0,"missing":0},
    "tags":{"matched":0,"created":0,"renamed":0,"reconciled":0,"skipped":0}
  },
  "warnings":[]
}`

const envBlockedResultBody = `{
  "environmentId":"` + testEnvironmentID + `",
  "git":{"commitSha":"` + selectionSHA + `","branchName":"main"},
  "status":"blocked",
  "preflight":{"missingProjects":[],"missingBindings":[],"accessRequirements":[],"conflicts":[],"warnings":[]}
}`

const envPromoteResultBody = `{
  "environmentId":"` + testEnvironmentID + `",
  "counts":{"workflows":2,"folders":1,"credentials":0,"dataTables":0,"variables":0,"tags":3},
  "git":{"commitSha":"deadbeef","branchName":"n8n/promotion-1"},
  "review":{"reviewId":"mr-1","number":42,"webUrl":"https://gitlab.example.com/org/repo/-/merge_requests/42"},
  "warnings":[{"message":"Branch protection blocks merging","linkUrl":"https://gitlab.example.com/help"},{"message":"No reviewers"}]
}`

func envSource() PromotionExpectedSource {
	return PromotionExpectedSource{EnvironmentID: testEnvironmentID, BranchName: "main", CommitSHA: selectionSHA}
}

func TestPromotionEnvironmentSyncTransport(t *testing.T) {
	yes := true
	message := "sync"
	path := PromotionEnvironmentsPath + "/" + testEnvironmentID
	cases := []struct {
		name, path, body, response string
		call                       func(context.Context, *Client) (any, error)
	}{
		{"apply without source", path + "/apply", `{}`, envAppliedResultBody, func(ctx context.Context, c *Client) (any, error) {
			return c.ApplyPromotionEnvironment(ctx, testEnvironmentID, ApplyPromotionEnvironmentRequest{})
		}},
		{"apply with source", path + "/apply", `{"expectedSource":` + envSourceJSON + `}`, envBlockedResultBody, func(ctx context.Context, c *Client) (any, error) {
			source := envSource()
			return c.ApplyPromotionEnvironment(ctx, testEnvironmentID, ApplyPromotionEnvironmentRequest{ExpectedSource: &source})
		}},
		{"continue", path + "/apply/continue", `{"expectedSource":` + envSourceJSON + `,"confirmDestructiveChanges":true}`, envAppliedResultBody, func(ctx context.Context, c *Client) (any, error) {
			return c.ContinueApplyPromotionEnvironment(ctx, testEnvironmentID, ContinueApplyPromotionEnvironmentRequest{ExpectedSource: envSource(), ConfirmDestructiveChanges: &yes})
		}},
		{"promote without message", path + "/promote", `{}`, envPromoteResultBody, func(ctx context.Context, c *Client) (any, error) {
			return c.PromotePromotionEnvironment(ctx, testEnvironmentID, PromotePromotionEnvironmentRequest{})
		}},
		{"promote with message and force", path + "/promote", `{"commitMessage":"sync","force":true}`, envPromoteResultBody, func(ctx context.Context, c *Client) (any, error) {
			return c.PromotePromotionEnvironment(ctx, testEnvironmentID, PromotePromotionEnvironmentRequest{CommitMessage: &message, Force: true})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, tc.response))
			result, err := tc.call(context.Background(), server.client(t))
			if err != nil {
				t.Fatal(err)
			}
			req, body := server.last(t)
			if req.Method != http.MethodPost || req.URL.EscapedPath() != BasePath+tc.path {
				t.Errorf("request = %s %s, want POST %s", req.Method, req.URL.EscapedPath(), BasePath+tc.path)
			}
			if body != tc.body {
				t.Errorf("body = %s, want %s", body, tc.body)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.response), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("response not preserved: %s", encoded)
			}
			for _, status := range []int{400, 403, 404, 409, 422, 503} {
				s := newCommunityPackageServer(t, packageJSONHandler(status, `{"message":"failure"}`))
				if _, err := tc.call(context.Background(), s.client(t)); !IsStatus(err, status) {
					t.Errorf("status %d: error = %v", status, err)
				}
			}
		})
	}
}

func TestPromotionEnvironmentSyncValidationBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, envAppliedResultBody))
	client := server.client(t)
	ctx := context.Background()
	configSource := selectionSource()
	both := envSource()
	both.ConfigID = "cfg"
	incomplete := envSource()
	incomplete.CommitSHA = "abc"
	blank := ""
	long := strings.Repeat("m", maxPromotionCommitLength+1)
	cases := []struct {
		name, want string
		call       func() error
	}{
		{"apply bad id", "must be a UUID", func() error {
			_, err := client.ApplyPromotionEnvironment(ctx, "conn-1", ApplyPromotionEnvironmentRequest{})
			return err
		}},
		{"apply config source", "configId, which only n8n 2.45", func() error {
			_, err := client.ApplyPromotionEnvironment(ctx, testEnvironmentID, ApplyPromotionEnvironmentRequest{ExpectedSource: &configSource})
			return err
		}},
		{"apply both ids", "not both", func() error {
			_, err := client.ApplyPromotionEnvironment(ctx, testEnvironmentID, ApplyPromotionEnvironmentRequest{ExpectedSource: &both})
			return err
		}},
		{"apply short commit", "full lowercase", func() error {
			_, err := client.ApplyPromotionEnvironment(ctx, testEnvironmentID, ApplyPromotionEnvironmentRequest{ExpectedSource: &incomplete})
			return err
		}},
		{"continue missing source", "--expected-environment-id", func() error {
			_, err := client.ContinueApplyPromotionEnvironment(ctx, testEnvironmentID, ContinueApplyPromotionEnvironmentRequest{})
			return err
		}},
		{"continue config source", "environmentId", func() error {
			_, err := client.ContinueApplyPromotionEnvironment(ctx, testEnvironmentID, ContinueApplyPromotionEnvironmentRequest{ExpectedSource: configSource})
			return err
		}},
		{"continue bad id", "whitespace", func() error {
			_, err := client.ContinueApplyPromotionEnvironment(ctx, " "+testEnvironmentID, ContinueApplyPromotionEnvironmentRequest{ExpectedSource: envSource()})
			return err
		}},
		{"promote blank message", "commit message is required", func() error {
			_, err := client.PromotePromotionEnvironment(ctx, testEnvironmentID, PromotePromotionEnvironmentRequest{CommitMessage: &blank})
			return err
		}},
		{"promote long message", "at most", func() error {
			_, err := client.PromotePromotionEnvironment(ctx, testEnvironmentID, PromotePromotionEnvironmentRequest{CommitMessage: &long})
			return err
		}},
		{"promote bad id", "must be a UUID", func() error {
			_, err := client.PromotePromotionEnvironment(ctx, "env", PromotePromotionEnvironmentRequest{})
			return err
		}},
		// Connection routes exist only before n8n 2.46, so an environment
		// source can never be right there.
		{"connection continue environment source", "n8n promotion environment apply-continue", func() error {
			_, err := client.ContinueApplyPackage(ctx, "conn-1", ContinueApplyPromotionRequest{ExpectedSource: envSource()})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want %q", err, tc.want)
			}
			if len(server.requests) != 0 {
				t.Fatalf("%d request(s) reached the instance", len(server.requests))
			}
		})
	}
}

// Project routes are served by both generations, so either identifier passes
// through as given.
func TestPromotionProjectSelectionBothExpectedSources(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source PromotionExpectedSource
		json   string
	}{
		{"n8n 2.45 configId", selectionSource(), selectionSourceJSON},
		{"n8n 2.46 environmentId", envSource(), envSourceJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, envAppliedResultBody))
			client := server.client(t)
			source := tc.source
			if _, err := client.ApplyProjectSelection(context.Background(), "p1", ApplyProjectSelectionRequest{WorkflowIDs: []string{"wf"}, ExpectedSource: &source}); err != nil {
				t.Fatal(err)
			}
			if _, body := server.last(t); body != `{"workflowIds":["wf"],"expectedSource":`+tc.json+`}` {
				t.Errorf("apply body = %s", body)
			}
			if _, err := client.ContinueApplyProjectSelection(context.Background(), "p1", ContinueApplyProjectSelectionRequest{WorkflowIDs: []string{"wf"}, ExpectedSource: tc.source}); err != nil {
				t.Fatal(err)
			}
			if _, body := server.last(t); body != `{"workflowIds":["wf"],"expectedSource":`+tc.json+`}` {
				t.Errorf("continue body = %s", body)
			}
		})
	}
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, envAppliedResultBody))
	if _, err := server.client(t).ContinueApplyProjectSelection(context.Background(), "p1", ContinueApplyProjectSelectionRequest{WorkflowIDs: []string{"wf"}}); err == nil ||
		!strings.Contains(err.Error(), "--expected-environment-id") || !strings.Contains(err.Error(), "--expected-config-id") {
		t.Errorf("missing source error = %v, want both flags named", err)
	}
	if len(server.requests) != 0 {
		t.Error("request without a source reached the instance")
	}
}

func TestApplyPromotionResultBothShapes(t *testing.T) {
	var applied ApplyPromotionResult
	if err := json.Unmarshal([]byte(envAppliedResultBody), &applied); err != nil {
		t.Fatal(err)
	}
	if applied.EnvironmentID != testEnvironmentID || applied.ConnectionID != "" || applied.ConfigID != "" || applied.Extra != nil {
		t.Errorf("identifiers = %+v, want only environmentId and no extras", applied)
	}
	if applied.SourceID() != testEnvironmentID || !applied.Applied() {
		t.Errorf("SourceID = %q applied = %v", applied.SourceID(), applied.Applied())
	}
	c := applied.Counts
	for name, got := range map[string]*int{"folders.updated": c.Folders.Updated, "folders.moved": c.Folders.Moved, "folders.renamed": c.Folders.Renamed, "workflows.moved": c.Workflows.Moved, "credentials.updated": c.Credentials.Updated} {
		want := map[string]int{"folders.updated": 3, "folders.moved": 2, "folders.renamed": 1, "workflows.moved": 2, "credentials.updated": 5}[name]
		if got == nil || *got != want {
			t.Errorf("%s = %v, want %d", name, got, want)
		}
	}

	var blocked ApplyPromotionResult
	if err := json.Unmarshal([]byte(envBlockedResultBody), &blocked); err != nil {
		t.Fatal(err)
	}
	if source := blocked.ExpectedSource(); source != envSource() || source.ValidateEnvironment() != nil {
		t.Errorf("ExpectedSource = %+v, want the environment source", source)
	}

	// n8n 2.45: connectionId and configId, no new counts.
	var old ApplyPromotionResult
	if err := json.Unmarshal([]byte(promotionApplyResultBody), &old); err != nil {
		t.Fatal(err)
	}
	if old.SourceID() != "conn-1" || old.ExpectedSource().ConfigID == "" || old.ExpectedSource().EnvironmentID != "" {
		t.Errorf("old shape = %+v", old)
	}
	if old.Counts != nil && (old.Counts.Folders.Updated != nil || old.Counts.Folders.Moved != nil || old.Counts.Folders.Renamed != nil || old.Counts.Workflows.Moved != nil || old.Counts.Credentials.Updated != nil) {
		t.Errorf("old shape invented 2.46 counts: %+v", *old.Counts)
	}
}

func TestPromotePromotionResultBothShapes(t *testing.T) {
	var result PromotePromotionResult
	if err := json.Unmarshal([]byte(envPromoteResultBody), &result); err != nil {
		t.Fatal(err)
	}
	if result.SourceID() != testEnvironmentID || result.ConnectionID != "" || result.ConfigID != "" {
		t.Errorf("identifiers = %+v", result)
	}
	if result.Review == nil || *result.Review != (PromotionReview{ReviewID: "mr-1", Number: 42, WebURL: "https://gitlab.example.com/org/repo/-/merge_requests/42"}) {
		t.Errorf("review = %+v", result.Review)
	}
	if len(result.Warnings) != 2 || result.Warnings[0].LinkURL == "" || result.Warnings[1].Message != "No reviewers" || result.Warnings[1].LinkURL != "" {
		t.Errorf("warnings = %+v", result.Warnings)
	}

	var old PromotePromotionResult
	if err := json.Unmarshal([]byte(promotionPromoteResultBody), &old); err != nil {
		t.Fatal(err)
	}
	if old.SourceID() != "conn-1" || old.Review != nil || old.Warnings != nil {
		t.Errorf("old shape = %+v", old)
	}
	encoded, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "environmentId") || !strings.Contains(string(encoded), `"configId":"cfg-promote"`) {
		t.Errorf("old shape re-encoded as %s, want the verbatim response", encoded)
	}
	// A constructed result names only the identifiers it has.
	encoded, err = json.Marshal(PromotePromotionResult{EnvironmentID: "e"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "connectionId") || strings.Contains(string(encoded), "configId") || strings.Contains(string(encoded), "review") {
		t.Errorf("constructed result = %s", encoded)
	}
}

func TestPromotionChangesEnvironmentSource(t *testing.T) {
	body := strings.Replace(selectionChangesJSON, `"configId":"cfg"`, `"environmentId":"`+testEnvironmentID+`"`, 1)
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))
	changes, err := server.client(t).GetPromotionChanges(context.Background(), "p1", PromotionDirectionApply, PromotionChangesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if changes.Source.EnvironmentID != testEnvironmentID || changes.Source.ConfigID != "" || changes.Source.BranchName != "main" {
		t.Errorf("source = %+v", changes.Source)
	}
}
