package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

const workflowResponse = `{
  "id":"wf-1",
  "name":"Invoice sync",
  "description":null,
  "active":true,
  "activeVersionId":"ver-1",
  "createdAt":"2026-09-16T20:03:43.517Z",
  "updatedAt":"2026-09-16T20:06:02.339Z",
  "isArchived":false,
  "versionId":"ver-1",
  "versionCounter":3,
  "sourceWorkflowId":null,
  "triggerCount":1,
  "nodes":[
    {"id":"n-1","name":"Webhook","type":"n8n-nodes-base.webhook","typeVersion":2,"position":[0,0],"parameters":{"path":"hook"}},
    {"id":"n-2","name":"Set","type":"n8n-nodes-base.set","typeVersion":3,"disabled":true,"position":[10,0],"parameters":{}}
  ],
  "connections":{"Webhook":{"main":[[{"node":"Set","type":"main","index":0}]]}},
  "nodeGroups":[],
  "settings":{"executionOrder":"v1"},
  "staticData":null,
  "pinData":{},
  "meta":{"templateCredsSetupCompleted":true},
  "tags":[{"id":"tag-1","name":"production"}],
  "shared":[{"role":"workflow:owner","projectId":"pr-1"}],
  "activeVersion":{"versionId":"ver-1"},
  "fieldAddedLater":{"keep":"me"}
}`

const workflowVersionResponse = `{
  "versionId":"ver-1",
  "workflowId":"wf-1",
  "authors":"Ada Lovelace",
  "name":null,
  "description":null,
  "createdAt":"2026-09-16T20:03:43.517Z",
  "updatedAt":"2026-09-16T20:03:43.517Z",
  "nodes":[{"id":"n-1","name":"Webhook","type":"n8n-nodes-base.webhook","typeVersion":2}],
  "connections":{},
  "nodeGroups":[]
}`

// writeDocument is a valid minimal create or update body.
func writeDocument() WorkflowDocument {
	return WorkflowDocument{
		"name":        json.RawMessage(`"Invoice sync"`),
		"nodes":       json.RawMessage(`[]`),
		"connections": json.RawMessage(`{}`),
		"settings":    json.RawMessage(`{"executionOrder":"v1"}`),
	}
}

func TestListWorkflowsQueryAndDecoding(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK,
		`{"data":[`+workflowResponse+`],"nextCursor":"next/one?x=1"}`))
	active := true
	page, err := server.client(t).ListWorkflows(context.Background(), ListWorkflowsOptions{
		ListOptions:       ListOptions{Limit: 50, Cursor: "prev/one?x=1"},
		Offset:            10,
		Active:            &active,
		Tags:              []string{"production", "finance"},
		Name:              "Invoice sync",
		ProjectID:         "pr-1",
		ExcludePinnedData: true,
	})
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+WorkflowsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want the API key", HeaderAPIKey, got)
	}
	query := req.URL.Query()
	for field, want := range map[string]string{
		"limit":             "50",
		"cursor":            "prev/one?x=1",
		"offset":            "10",
		"active":            "true",
		"tags":              "production,finance",
		"name":              "Invoice sync",
		"projectId":         "pr-1",
		"excludePinnedData": "true",
	} {
		if got := query.Get(field); got != want {
			t.Errorf("query %s = %q, want %q", field, got, want)
		}
	}

	if len(page.Data) != 1 || page.NextCursor != "next/one?x=1" {
		t.Fatalf("page = %+v", page)
	}
	workflow := page.Data[0]
	if workflow.ID != "wf-1" || workflow.Name != "Invoice sync" || !workflow.Active || workflow.IsArchived {
		t.Errorf("workflow = %+v", workflow)
	}
	if workflow.VersionCounter != 3 || workflow.TriggerCount != 1 || workflow.ActiveVersionID != "ver-1" {
		t.Errorf("workflow versions = %+v", workflow)
	}
	if len(workflow.Tags) != 1 || workflow.Tags[0].Name != "production" {
		t.Errorf("tags = %+v", workflow.Tags)
	}
	if got := workflow.NodeCount(); got != 2 {
		t.Errorf("NodeCount = %d, want 2", got)
	}
	nodes := workflow.NodeSummaries()
	if nodes[0].Name != "Webhook" || nodes[0].Type != "n8n-nodes-base.webhook" || nodes[1].Disabled != true {
		t.Errorf("node summaries = %+v", nodes)
	}
}

func TestListWorkflowsDefaultQueryAndValidation(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))
	client := server.client(t)
	if _, err := client.ListWorkflows(context.Background(), ListWorkflowsOptions{}); err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if got := server.requests[0].URL.RawQuery; got != "" {
		t.Errorf("query = %q, want empty", got)
	}

	for name, opts := range map[string]ListWorkflowsOptions{
		"negative limit":  {ListOptions: ListOptions{Limit: -1}},
		"negative offset": {Offset: -1},
		"empty tag":       {Tags: []string{" "}},
		"comma in tag":    {Tags: []string{"a,b"}},
	} {
		if _, err := client.ListWorkflows(context.Background(), opts); err == nil {
			t.Errorf("%s: want an error before transport", name)
		}
	}
	if len(server.requests) != 1 {
		t.Errorf("%d requests reached the instance, want only the valid one", len(server.requests))
	}
}

func TestWorkflowRoundTripKeepsUnknownFields(t *testing.T) {
	var workflow Workflow
	if err := json.Unmarshal([]byte(workflowResponse), &workflow); err != nil {
		t.Fatalf("decode workflow: %v", err)
	}
	if len(workflow.Extra) != 1 {
		t.Fatalf("Extra = %v, want the one undocumented field", workflow.Extra)
	}
	encoded, err := json.Marshal(workflow)
	if err != nil {
		t.Fatalf("encode workflow: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("re-decode workflow: %v", err)
	}
	assertJSONEqual(t, `{"keep":"me"}`, string(got["fieldAddedLater"]))
	assertJSONEqual(t, `{"Webhook":{"main":[[{"node":"Set","type":"main","index":0}]]}}`, string(got["connections"]))
	if _, ok := got["nodes"]; !ok {
		t.Error("the re-encoded workflow lost its nodes")
	}

	doc, err := workflow.Document()
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if doc.Text("name") != "Invoice sync" {
		t.Errorf("document name = %q", doc.Text("name"))
	}
}

func TestWorkflowDocumentForWriteDropsReadOnlyFields(t *testing.T) {
	var workflow Workflow
	if err := json.Unmarshal([]byte(workflowResponse), &workflow); err != nil {
		t.Fatalf("decode workflow: %v", err)
	}
	doc, err := workflow.Document()
	if err != nil {
		t.Fatalf("Document: %v", err)
	}

	update, dropped := doc.ForUpdate()
	for _, field := range []string{"id", "active", "versionId", "createdAt", "tags", "shared", "activeVersion", "meta"} {
		if _, ok := update[field]; ok {
			t.Errorf("update body still carries the read-only field %q", field)
		}
	}
	for _, field := range []string{"name", "nodes", "connections", "settings", "pinData"} {
		if _, ok := update[field]; !ok {
			t.Errorf("update body lost the writable field %q", field)
		}
	}
	// staticData is null in the response and documented as nullable, so it is
	// still sent.
	if _, ok := update["staticData"]; !ok {
		t.Error("update body dropped the nullable staticData field")
	}
	if !contains(dropped, "id") || !contains(dropped, "fieldAddedLater") {
		t.Errorf("dropped = %v, want it to name id and the undocumented field", dropped)
	}
	if err := update.Validate(); err != nil {
		t.Errorf("Validate after filtering: %v", err)
	}

	// A null typed as a string is what the instance rejects with
	// "Expected string, received null", so it is dropped rather than sent.
	nulls := WorkflowDocument{
		"name":        json.RawMessage(`"x"`),
		"description": json.RawMessage(`null`),
		"nodes":       json.RawMessage(`[]`),
		"connections": json.RawMessage(`{}`),
		"settings":    json.RawMessage(`{}`),
		"pinData":     json.RawMessage(`null`),
	}
	filtered, nullsDropped := nulls.ForUpdate()
	if _, ok := filtered["description"]; ok {
		t.Error("update body kept a null description, which the API rejects")
	}
	if _, ok := filtered["pinData"]; !ok {
		t.Error("update body dropped a null pinData, which the API documents as nullable")
	}
	if !contains(nullsDropped, "description") {
		t.Errorf("dropped = %v, want it to name description", nullsDropped)
	}

	create, _ := doc.ForCreate()
	if _, ok := create["description"]; ok {
		t.Error("create body carries description, which POST /workflows does not accept")
	}
	if err := create.Set("projectId", "pr-2"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if create.Text("projectId") != "pr-2" {
		t.Errorf("projectId = %q", create.Text("projectId"))
	}
}

func TestWorkflowDocumentValidate(t *testing.T) {
	tests := []struct {
		name string
		doc  WorkflowDocument
		want string
	}{
		{name: "complete", doc: writeDocument()},
		{name: "missing nodes", doc: WorkflowDocument{
			"name":        json.RawMessage(`"x"`),
			"connections": json.RawMessage(`{}`),
			"settings":    json.RawMessage(`{}`),
		}, want: "nodes"},
		{name: "empty name", doc: WorkflowDocument{
			"name":        json.RawMessage(`"  "`),
			"nodes":       json.RawMessage(`[]`),
			"connections": json.RawMessage(`{}`),
			"settings":    json.RawMessage(`{}`),
		}, want: "name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.doc.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate error = %v, want it to name %q", err, tt.want)
			}
		})
	}
}

func TestWorkflowRequests(t *testing.T) {
	id := "wf/one?x=1"
	versionID := "ver/two?y=2"
	publishOff := false
	tests := []struct {
		name   string
		call   func(*Client) error
		method string
		path   string
		query  string
		body   string
		status int
		result string
	}{
		{name: "create", call: func(c *Client) error {
			workflow, err := c.CreateWorkflow(context.Background(), writeDocument())
			if err == nil && workflow.ID != "wf-1" {
				t.Errorf("workflow = %+v", workflow)
			}
			return err
		}, method: http.MethodPost, path: "/workflows", status: http.StatusOK, result: workflowResponse,
			body: `{"name":"Invoice sync","nodes":[],"connections":{},"settings":{"executionOrder":"v1"}}`},

		{name: "get", call: func(c *Client) error {
			_, err := c.GetWorkflow(context.Background(), id, false)
			return err
		}, method: http.MethodGet, path: "/workflows/wf%2Fone%3Fx=1", status: http.StatusOK, result: workflowResponse},

		{name: "get without pinned data", call: func(c *Client) error {
			_, err := c.GetWorkflow(context.Background(), id, true)
			return err
		}, method: http.MethodGet, path: "/workflows/wf%2Fone%3Fx=1", query: "excludePinnedData=true",
			status: http.StatusOK, result: workflowResponse},

		{name: "update", call: func(c *Client) error {
			_, err := c.UpdateWorkflow(context.Background(), id, writeDocument(), UpdateWorkflowOptions{})
			return err
		}, method: http.MethodPut, path: "/workflows/wf%2Fone%3Fx=1", status: http.StatusOK, result: workflowResponse,
			body: `{"name":"Invoice sync","nodes":[],"connections":{},"settings":{"executionOrder":"v1"}}`},

		{name: "update as draft", call: func(c *Client) error {
			_, err := c.UpdateWorkflow(context.Background(), id, writeDocument(), UpdateWorkflowOptions{PublishIfActive: &publishOff})
			return err
		}, method: http.MethodPut, path: "/workflows/wf%2Fone%3Fx=1", query: "publishIfActive=false",
			status: http.StatusOK, result: workflowResponse,
			body: `{"name":"Invoice sync","nodes":[],"connections":{},"settings":{"executionOrder":"v1"}}`},

		{name: "delete", call: func(c *Client) error {
			_, err := c.DeleteWorkflow(context.Background(), id)
			return err
		}, method: http.MethodDelete, path: "/workflows/wf%2Fone%3Fx=1", status: http.StatusOK, result: workflowResponse},

		{name: "archive", call: func(c *Client) error {
			_, err := c.ArchiveWorkflow(context.Background(), id)
			return err
		}, method: http.MethodPost, path: "/workflows/wf%2Fone%3Fx=1/archive", status: http.StatusOK, result: workflowResponse},

		{name: "unarchive", call: func(c *Client) error {
			_, err := c.UnarchiveWorkflow(context.Background(), id)
			return err
		}, method: http.MethodPost, path: "/workflows/wf%2Fone%3Fx=1/unarchive", status: http.StatusOK, result: workflowResponse},

		{name: "publish latest", call: func(c *Client) error {
			_, err := c.PublishWorkflow(context.Background(), id, PublishWorkflowRequest{})
			return err
		}, method: http.MethodPost, path: "/workflows/wf%2Fone%3Fx=1/publish", status: http.StatusOK, result: workflowResponse,
			body: `{}`},

		{name: "publish a version", call: func(c *Client) error {
			_, err := c.PublishWorkflow(context.Background(), id, PublishWorkflowRequest{
				VersionID: versionID, Name: "Release", Description: "adds the retry branch",
			})
			return err
		}, method: http.MethodPost, path: "/workflows/wf%2Fone%3Fx=1/publish", status: http.StatusOK, result: workflowResponse,
			body: `{"versionId":"ver/two?y=2","name":"Release","description":"adds the retry branch"}`},

		{name: "unpublish", call: func(c *Client) error {
			_, err := c.UnpublishWorkflow(context.Background(), id)
			return err
		}, method: http.MethodPost, path: "/workflows/wf%2Fone%3Fx=1/unpublish", status: http.StatusOK, result: workflowResponse},

		{name: "activate, deprecated", call: func(c *Client) error {
			_, err := c.ActivateWorkflow(context.Background(), id, PublishWorkflowRequest{})
			return err
		}, method: http.MethodPost, path: "/workflows/wf%2Fone%3Fx=1/activate", status: http.StatusOK, result: workflowResponse,
			body: `{}`},

		{name: "deactivate, deprecated", call: func(c *Client) error {
			_, err := c.DeactivateWorkflow(context.Background(), id)
			return err
		}, method: http.MethodPost, path: "/workflows/wf%2Fone%3Fx=1/deactivate", status: http.StatusOK, result: workflowResponse},

		{name: "transfer", call: func(c *Client) error {
			return c.TransferWorkflow(context.Background(), id, "pr-2")
		}, method: http.MethodPut, path: "/workflows/wf%2Fone%3Fx=1/transfer", status: http.StatusNoContent,
			body: `{"destinationProjectId":"pr-2"}`},

		{name: "history", call: func(c *Client) error {
			page, err := c.ListWorkflowHistory(context.Background(), id, ListOptions{Limit: 2})
			if err == nil && (len(page.Data) != 1 || page.Data[0].VersionID != "ver-1" || page.Data[0].Authors != "Ada Lovelace") {
				t.Errorf("history = %+v", page)
			}
			return err
		}, method: http.MethodGet, path: "/workflows/wf%2Fone%3Fx=1/history", query: "limit=2", status: http.StatusOK,
			result: `{"data":[{"versionId":"ver-1","workflowId":"wf-1","authors":"Ada Lovelace","name":null,"description":null,"createdAt":"2026-09-16T20:03:43.517Z","updatedAt":"2026-09-16T20:03:43.517Z"}],"nextCursor":null}`},

		{name: "version get", call: func(c *Client) error {
			version, err := c.GetWorkflowVersion(context.Background(), id, versionID)
			if err == nil && (version.VersionID != "ver-1" || version.Authors != "Ada Lovelace") {
				t.Errorf("version = %+v", version)
			}
			return err
		}, method: http.MethodGet, path: "/workflows/wf%2Fone%3Fx=1/versions/ver%2Ftwo%3Fy=2",
			status: http.StatusOK, result: workflowVersionResponse},

		{name: "version get through the deprecated route", call: func(c *Client) error {
			_, err := c.GetWorkflowVersionLegacy(context.Background(), id, versionID)
			return err
		}, method: http.MethodGet, path: "/workflows/wf%2Fone%3Fx=1/ver%2Ftwo%3Fy=2",
			status: http.StatusOK, result: workflowVersionResponse},

		{name: "tag list", call: func(c *Client) error {
			tags, err := c.GetWorkflowTags(context.Background(), id)
			if err == nil && (len(tags) != 1 || tags[0].ID != "tag-1") {
				t.Errorf("tags = %+v", tags)
			}
			return err
		}, method: http.MethodGet, path: "/workflows/wf%2Fone%3Fx=1/tags", status: http.StatusOK,
			result: `[{"id":"tag-1","name":"production"}]`},

		{name: "tag set", call: func(c *Client) error {
			_, err := c.SetWorkflowTags(context.Background(), id, []string{"tag-1", "tag-2"})
			return err
		}, method: http.MethodPut, path: "/workflows/wf%2Fone%3Fx=1/tags", status: http.StatusOK,
			body: `[{"id":"tag-1"},{"id":"tag-2"}]`, result: `[{"id":"tag-1","name":"production"}]`},

		{name: "tag clear", call: func(c *Client) error {
			_, err := c.SetWorkflowTags(context.Background(), id, nil)
			return err
		}, method: http.MethodPut, path: "/workflows/wf%2Fone%3Fx=1/tags", status: http.StatusOK,
			body: `[]`, result: `[]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tt.status, tt.result))
			if err := tt.call(server.client(t)); err != nil {
				t.Fatalf("call: %v", err)
			}
			req, body := server.last(t)
			if req.Method != tt.method {
				t.Errorf("method = %q, want %q", req.Method, tt.method)
			}
			if want := BasePath + tt.path; req.URL.EscapedPath() != want {
				t.Errorf("path = %q, want %q", req.URL.EscapedPath(), want)
			}
			if req.URL.RawQuery != tt.query {
				t.Errorf("query = %q, want %q", req.URL.RawQuery, tt.query)
			}
			if tt.body == "" {
				if body != "" {
					t.Errorf("body = %q, want empty", body)
				}
				return
			}
			assertJSONEqual(t, tt.body, body)
		})
	}
}

func TestWorkflowValidationHappensBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, workflowResponse))
	client := server.client(t)

	calls := map[string]func() error{
		"empty ID": func() error {
			_, err := client.GetWorkflow(context.Background(), "  ", false)
			return err
		},
		"padded ID": func() error {
			_, err := client.ArchiveWorkflow(context.Background(), " wf-1 ")
			return err
		},
		"incomplete create": func() error {
			_, err := client.CreateWorkflow(context.Background(), WorkflowDocument{"name": json.RawMessage(`"x"`)})
			return err
		},
		"incomplete update": func() error {
			_, err := client.UpdateWorkflow(context.Background(), "wf-1", WorkflowDocument{}, UpdateWorkflowOptions{})
			return err
		},
		"transfer without a destination": func() error {
			return client.TransferWorkflow(context.Background(), "wf-1", "")
		},
		"empty version ID": func() error {
			_, err := client.GetWorkflowVersion(context.Background(), "wf-1", "")
			return err
		},
		"empty tag ID": func() error {
			_, err := client.SetWorkflowTags(context.Background(), "wf-1", []string{"tag-1", ""})
			return err
		},
		"duplicate tag ID": func() error {
			_, err := client.SetWorkflowTags(context.Background(), "wf-1", []string{"tag-1", "tag-1"})
			return err
		},
		"negative history limit": func() error {
			_, err := client.ListWorkflowHistory(context.Background(), "wf-1", ListOptions{Limit: -1})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: want an error before transport", name)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("%d requests reached the instance, want none", len(server.requests))
	}
}

func TestWorkflowAPIErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"message":"unauthorized"}`, check: IsUnauthorized},
		{name: "forbidden", status: http.StatusForbidden, body: `{"message":"missing workflow:activate"}`, check: IsForbidden},
		{name: "not found", status: http.StatusNotFound, body: `{"message":"Not Found"}`, check: IsNotFound},
		{name: "conflict", status: http.StatusConflict, body: `{"message":"blocked","reason":"review","workflowReviewRequestId":"rev-1"}`, check: IsConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tt.status, tt.body))
			_, err := server.client(t).PublishWorkflow(context.Background(), "wf-1", PublishWorkflowRequest{})
			if err == nil {
				t.Fatal("PublishWorkflow: want an error")
			}
			if !tt.check(err) {
				t.Errorf("error = %v, want status %d", err, tt.status)
			}
		})
	}
}

func TestWorkflowUpdateKeeps403Distinguishable(t *testing.T) {
	// A 403 from an update is a partial success: the draft was saved and the
	// published version stayed live, so the caller has to tell it apart.
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusForbidden,
		`{"message":"User is missing the workflow:publish permission"}`))
	_, err := server.client(t).UpdateWorkflow(context.Background(), "wf-1", writeDocument(), UpdateWorkflowOptions{})
	if !IsForbidden(err) {
		t.Fatalf("error = %v, want a 403", err)
	}
	if !strings.Contains(err.Error(), "workflow:publish") {
		t.Errorf("error = %v, want the API message preserved", err)
	}
}

func TestCollectWorkflowsFollowsCursors(t *testing.T) {
	pages := []Page[Workflow]{
		{Data: []Workflow{{ID: "wf-1"}}, NextCursor: "c1"},
		{Data: []Workflow{{ID: "wf-2"}}},
	}
	var seen []string
	fetch := func(_ context.Context, opts ListOptions) (Page[Workflow], error) {
		seen = append(seen, opts.Cursor)
		page := pages[0]
		pages = pages[1:]
		return page, nil
	}
	workflows, err := Collect(context.Background(), fetch, ListOptions{Limit: 1}, 0)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(workflows) != 2 || workflows[1].ID != "wf-2" {
		t.Errorf("workflows = %+v", workflows)
	}
	if len(seen) != 2 || seen[0] != "" || seen[1] != "c1" {
		t.Errorf("cursors = %v", seen)
	}
}

func contains(values []string, want string) bool {
	return slices.Contains(values, want)
}
