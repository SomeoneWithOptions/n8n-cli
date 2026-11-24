package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const preview249Revision = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

// promotionChanges249JSON is an n8n 2.49 Apply preview: resources with
// context, dependencies, the Apply check with moved workflows, and the
// preview revision.
const promotionChanges249JSON = `{
  "commitSha":"` + selectionSHA + `",
  "source":{"environmentId":"env-1","branchName":"main"},
  "resources":[
    {"key":"project:p1","type":"project","id":"p1","name":"Ops","parentKey":null,"change":"unchanged","selectable":false,"revision":{"before":"r","after":"r"}},
    {"key":"folder:f1","type":"folder","id":"f1","name":"Billing","previousName":"Bills","parentKey":"project:p1","change":"modified","selectable":true,"revision":{"before":"a","after":"b"}},
    {"key":"workflow:w1","type":"workflow","id":"w1","name":"Invoice","parentKey":"folder:f1","change":"added","selectable":true,"movedFromProject":{"id":"p2","name":"Sales"},"revision":{"before":null,"after":"c"}},
    {"key":"credential:c1","type":"credential","id":"c1","name":"Stripe","parentKey":null,"change":"modified","selectable":false,"revision":{"before":"d","after":"e"},"future":1}
  ],
  "dependencies":[{"workflowKey":"workflow:w1","resourceKey":"credential:c1","included":true}],
  "check":{"missingProjects":[],"missingBindings":[],"accessRequirements":[],"warnings":[],
    "conflicts":[{"kind":"tag","code":"tag-conflict","id":"t1","name":"prod","reason":"name-collision","consumers":[{"project":{"id":"p1","name":"Ops"},"workflows":[{"id":"w1","name":"Invoice"}]}]}],
    "movedWorkflows":[{"id":"w9","name":"Moved","project":{"id":"p2","name":"Sales"},"projectExists":false,"instanceProject":{"id":"p1"}}],
    "movedFolders":[]},
  "revision":"` + preview249Revision + `"
}`

func TestGetPromotionChangesResourceView(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, promotionChanges249JSON))
	result, err := server.client(t).GetPromotionChanges(context.Background(), "p1", "apply", PromotionChangesOptions{})
	if err != nil {
		t.Fatalf("GetPromotionChanges: %v", err)
	}
	req, _ := server.last(t)
	if req.URL.RawQuery != "" {
		t.Errorf("query = %q, want no filters by default (n8n 2.49 dropped them)", req.URL.RawQuery)
	}
	if !result.ResourceView() || len(result.Resources) != 4 || result.Revision != preview249Revision || result.Source.EnvironmentID != "env-1" {
		t.Fatalf("result = %+v", result)
	}
	changed := result.Changed()
	if len(changed) != 3 || changed[0].Key != "folder:f1" {
		t.Errorf("changed = %+v", changed)
	}
	workflow := result.Resources[2]
	if workflow.MovedFromProject == nil || workflow.MovedFromProject.Name != "Sales" || workflow.Revision.Before != nil || *workflow.ParentKey != "folder:f1" {
		t.Errorf("workflow = %+v", workflow)
	}
	if len(result.Dependencies) != 1 || !result.Dependencies[0].Included {
		t.Errorf("dependencies = %+v", result.Dependencies)
	}
	check := result.Check
	if check == nil || check.Empty() || len(check.Conflicts) != 1 || check.Conflicts[0].Reason != "name-collision" ||
		len(check.MovedWorkflows) != 1 || check.MovedWorkflows[0].ProjectExists {
		t.Errorf("check = %+v", check)
	}
	out, _ := json.Marshal(result)
	if !strings.Contains(string(out), `"future":1`) {
		t.Errorf("marshal dropped unknown fields: %s", out)
	}

	var legacy PromotionChanges
	if err := json.Unmarshal([]byte(selectionChangesJSON), &legacy); err != nil || legacy.ResourceView() {
		t.Errorf("legacy preview: ResourceView = %v, err %v", legacy.ResourceView(), err)
	}
	var promote PromotionChanges
	if err := json.Unmarshal([]byte(`{"commitSha":null,"source":{"environmentId":"e","branchName":"main"},"resources":[],"dependencies":[],"blocker":{"code":"reviewOpen","reviewId":"42"}}`), &promote); err != nil ||
		!promote.ResourceView() || promote.Blocker == nil || promote.Blocker.Code != PromotionBlockerReviewOpen {
		t.Errorf("promote preview = %+v, err %v", promote, err)
	}
}

func TestProjectSelectionFoldersAndEmptySelection(t *testing.T) {
	message := "sync"
	cases := []struct {
		name, path, body string
		call             func(context.Context, *Client) error
	}{
		{"apply folders", "/promotions/projects/p1/apply", `{"folderIds":["f1"],"expectedPreviewRevision":"` + preview249Revision + `"}`, func(ctx context.Context, c *Client) error {
			_, err := c.ApplyProjectSelection(ctx, "p1", ApplyProjectSelectionRequest{FolderIDs: []string{"f1"}, ExpectedPreviewRevision: preview249Revision})
			return err
		}},
		{"apply project only", "/promotions/projects/p1/apply", `{}`, func(ctx context.Context, c *Client) error {
			_, err := c.ApplyProjectSelection(ctx, "p1", ApplyProjectSelectionRequest{})
			return err
		}},
		{"continue folders", "/promotions/projects/p1/apply/continue", `{"workflowIds":["w1"],"folderIds":["f1"],"expectedSource":` + selectionSourceJSON + `}`, func(ctx context.Context, c *Client) error {
			_, err := c.ContinueApplyProjectSelection(ctx, "p1", ContinueApplyProjectSelectionRequest{WorkflowIDs: []string{"w1"}, FolderIDs: []string{"f1"}, ExpectedSource: selectionSource()})
			return err
		}},
		{"promote project only", "/promotions/projects/p1/promote", `{"commitMessage":"sync"}`, func(ctx context.Context, c *Client) error {
			_, err := c.PromoteProjectSelection(ctx, "p1", PromoteProjectSelectionRequest{CommitMessage: &message})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := `{"status":"applied","environmentId":"env-1","git":{"branchName":"main","commitSha":"` + selectionSHA + `"},"counts":{},"warnings":[]}`
			if strings.HasPrefix(tc.name, "promote") {
				response = promotionPromoteResultBody
			}
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, response))
			if err := tc.call(context.Background(), server.client(t)); err != nil {
				t.Fatal(err)
			}
			req, body := server.last(t)
			if req.Method != http.MethodPost || req.URL.Path != BasePath+tc.path || body != tc.body {
				t.Errorf("request = %s %s body %s, want %s", req.Method, req.URL.Path, body, tc.body)
			}
		})
	}
	if !EmptyPromotionSelection(nil, []string{}) || EmptyPromotionSelection(nil, []string{"f"}) {
		t.Error("EmptyPromotionSelection")
	}
}

const changeDetailsJSON = `{
  "details":[
    {"key":"workflow:w1","type":"workflow","before":null,"after":{"id":"w1","name":"Invoice","nodes":[]},"revision":{"before":null,"after":"c"}},
    {"key":"dataTable:d1","type":"dataTable","before":{"id":"d1","name":"Orders","columns":[]},"after":{"id":"d1","name":"Orders","columns":[]},"revision":{"before":"x","after":"y"},
     "dataTableChanges":[{"kind":"remove-column","column":"total","type":"number","destructive":true}]},
    {"key":"variable:v1","type":"variable","before":{"name":"TOKEN","type":"string"},"after":null,"revision":{"before":"v","after":null},"valueHidden":true}
  ],
  "missingKeys":["tag:t9"]
}`

func TestGetPromotionChangeDetails(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, changeDetailsJSON))
	client := server.client(t)
	sha := selectionSHA
	result, err := client.GetPromotionChangeDetails(context.Background(), "p/1", "promote", PromotionChangeDetailsRequest{CommitSHA: &sha, Keys: []string{"workflow:w1", "dataTable:d1"}})
	if err != nil {
		t.Fatalf("GetPromotionChangeDetails: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPost || req.URL.EscapedPath() != BasePath+"/promotions/projects/p%2F1/changes/promote/details" ||
		body != `{"commitSha":"`+selectionSHA+`","keys":["workflow:w1","dataTable:d1"]}` {
		t.Errorf("request = %s %s body %s", req.Method, req.URL.EscapedPath(), body)
	}
	if len(result.Details) != 3 || len(result.MissingKeys) != 1 {
		t.Fatalf("result = %+v", result)
	}
	added, table, variable := result.Details[0], result.Details[1], result.Details[2]
	if added.Change() != PromotionResourceAdded || added.Name() != "Invoice" {
		t.Errorf("added = %s %q", added.Change(), added.Name())
	}
	if table.Change() != PromotionResourceModified || len(table.DataTableChanges) != 1 || !table.DataTableChanges[0].Destructive {
		t.Errorf("table = %+v", table)
	}
	if variable.Change() != PromotionResourceDeleted || !variable.ValueHidden || variable.Name() != "TOKEN" {
		t.Errorf("variable = %+v", variable)
	}
	out, _ := json.Marshal(result)
	var a, b bytes.Buffer
	_ = json.Compact(&a, out)
	_ = json.Compact(&b, []byte(changeDetailsJSON))
	if a.String() != b.String() {
		t.Errorf("marshal = %s, want the response verbatim", out)
	}

	// A preview without a commit sends commitSha null.
	if _, err := client.GetPromotionChangeDetails(context.Background(), "p1", "apply", PromotionChangeDetailsRequest{Keys: []string{"k"}}); err != nil {
		t.Fatal(err)
	}
	if _, body := server.last(t); body != `{"commitSha":null,"keys":["k"]}` {
		t.Errorf("null commit body = %s", body)
	}
}

func TestGetPromotionChangeDetailsValidatesBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, changeDetailsJSON))
	client := server.client(t)
	short := "abc"
	upper := strings.ToUpper(selectionSHA[:39]) + "A"
	many := make([]string, MaxPromotionChangeDetailKeys+1)
	for i := range many {
		many[i] = "k" + strings.Repeat("x", i)
	}
	for _, tc := range []struct {
		project, direction string
		request            PromotionChangeDetailsRequest
	}{
		{"p", "pull", PromotionChangeDetailsRequest{Keys: []string{"k"}}},
		{"", "apply", PromotionChangeDetailsRequest{Keys: []string{"k"}}},
		{"p", "apply", PromotionChangeDetailsRequest{}},
		{"p", "apply", PromotionChangeDetailsRequest{Keys: many}},
		{"p", "apply", PromotionChangeDetailsRequest{Keys: []string{""}}},
		{"p", "apply", PromotionChangeDetailsRequest{Keys: []string{"k", "k"}}},
		{"p", "apply", PromotionChangeDetailsRequest{CommitSHA: &short, Keys: []string{"k"}}},
		{"p", "apply", PromotionChangeDetailsRequest{CommitSHA: &upper, Keys: []string{"k"}}},
	} {
		if _, err := client.GetPromotionChangeDetails(context.Background(), tc.project, tc.direction, tc.request); err == nil {
			t.Errorf("%+v accepted", tc)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("validation sent %d requests", len(server.requests))
	}
}

func TestIsLocked(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusLocked, `{"message":"The resource is locked by another operation."}`))
	_, err := server.client(t).ApplyProjectSelection(context.Background(), "p1", ApplyProjectSelectionRequest{WorkflowIDs: []string{"w"}})
	if !IsLocked(err) || IsConflict(err) {
		t.Errorf("err = %v, want a 423", err)
	}
}

// TestExportPackageIncludeArchivedFallback covers instances before n8n 2.49,
// which reject includeArchived as an unrecognized key and accept
// includeArchivedWorkflows.
func TestExportPackageIncludeArchivedFallback(t *testing.T) {
	legacyRejection := `{"code":"unrecognized_keys","keys":["includeArchived"],"path":[],"message":"Unrecognized key(s) in object: 'includeArchived'"}`
	var n int
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			packageJSONHandler(http.StatusBadRequest, legacyRejection)(w, r)
			return
		}
		exportGzipHandler(t, `{"workflows":1}`, "")(w, r)
	})
	var out bytes.Buffer
	result, err := server.client(t).ExportPackage(context.Background(), ExportPackageRequest{ProjectIDs: []string{"p1"}, IncludeArchived: Bool(true)}, &out)
	if err != nil {
		t.Fatalf("ExportPackage: %v", err)
	}
	if len(server.requests) != 2 || result.Counts["workflows"] != 1 || out.String() != packageArchiveBytes {
		t.Fatalf("requests = %d, result = %+v", len(server.requests), result)
	}
	if server.bodies[0] != `{"projectIds":["p1"],"includeArchived":true}` {
		t.Errorf("first body = %s", server.bodies[0])
	}
	if server.bodies[1] != `{"includeArchivedWorkflows":true,"projectIds":["p1"]}` {
		t.Errorf("legacy body = %s", server.bodies[1])
	}
}

func TestExportPackageNoFallbackForOtherErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request ExportPackageRequest
		body    string
	}{
		{"flag unset", ExportPackageRequest{ProjectIDs: []string{"p1"}}, `{"code":"unrecognized_keys","keys":["includeArchived"],"path":[]}`},
		{"other key", ExportPackageRequest{ProjectIDs: []string{"p1"}, IncludeArchived: Bool(true)}, `{"code":"unrecognized_keys","keys":["somethingElse"],"path":[]}`},
		{"other 400", ExportPackageRequest{ProjectIDs: []string{"p1"}, IncludeArchived: Bool(false)}, `{"message":"Project not found"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusBadRequest, tc.body))
			var out bytes.Buffer
			if _, err := server.client(t).ExportPackage(context.Background(), tc.request, &out); err == nil {
				t.Fatal("want the 400")
			}
			if len(server.requests) != 1 {
				t.Errorf("requests = %d, want no resend", len(server.requests))
			}
		})
	}
}
