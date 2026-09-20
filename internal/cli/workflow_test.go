package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliWorkflow = `{
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
  "triggerCount":1,
  "nodes":[{"id":"n-1","name":"Webhook","type":"n8n-nodes-base.webhook","typeVersion":2,"parameters":{"path":"hook"}}],
  "connections":{},
  "settings":{"executionOrder":"v1"},
  "staticData":null,
  "pinData":{},
  "tags":[{"id":"tag-1","name":"production"}]
}`

func workflowFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

// writeWorkflowFile stages a definition on disk, the way a user would after
// 'n8n workflow get --output json > workflow.json'.
func writeWorkflowFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "workflow.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow file: %v", err)
	}
	return path
}

func TestWorkflowListTextJSONAndQuery(t *testing.T) {
	f := workflowFixture(t, `{"data":[`+cliWorkflow+`],"nextCursor":"next/one?x=1"}`)
	got := f.run("workflow", "list", "--limit", "50", "--cursor", "prev/one?x=1", "--offset", "10",
		"--active", "true", "--tag", "production", "--tag", "finance", "--name", "Invoice sync",
		"--project-id", "pr-1", "--exclude-pinned-data")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Workflows:", "wf-1", "Invoice sync", "production", "Next cursor:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.WorkflowsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	query := req.URL.Query()
	for field, want := range map[string]string{
		"limit": "50", "cursor": "prev/one?x=1", "offset": "10", "active": "true",
		"tags": "production,finance", "name": "Invoice sync", "projectId": "pr-1",
		"excludePinnedData": "true",
	} {
		if got := query.Get(field); got != want {
			t.Errorf("query %s = %q, want %q", field, got, want)
		}
	}

	got = f.run("workflow", "list", "--output", "json")
	var page n8n.Page[n8n.Workflow]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 || page.Data[0].ID != "wf-1" {
		t.Errorf("stdout = %q, want a workflow page (error: %v)", got.stdout, err)
	}
}

func TestWorkflowListRejectsUnknownActiveValue(t *testing.T) {
	f := workflowFixture(t, `{"data":[],"nextCursor":null}`)
	before := f.requestCount()
	got := f.run("workflow", "list", "--active", "maybe")
	if got.code != ExitError || !strings.Contains(got.stderr, "use true or false") {
		t.Errorf("result = %+v", got)
	}
	if f.requestCount() != before {
		t.Error("an invalid --active value reached the instance")
	}
}

func TestWorkflowListAllFollowsCursor(t *testing.T) {
	f := workflowFixture(t, "")
	f.bodyFunc = func(page int) string {
		if page == 0 {
			return `{"data":[` + cliWorkflow + `],"nextCursor":"next"}`
		}
		return `{"data":[{"id":"wf-2","name":"Other"}],"nextCursor":null}`
	}
	got := f.run("workflow", "list", "--all", "--limit", "1", "--name", "Invoice sync", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.Workflow]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 2 {
		t.Errorf("stdout = %q, want both pages collected (error: %v)", got.stdout, err)
	}
	second := f.lastRequest()
	if second.URL.Query().Get("cursor") != "next" || second.URL.Query().Get("name") != "Invoice sync" {
		t.Errorf("second page query = %q, want the cursor with the filter preserved", second.URL.RawQuery)
	}
}

func TestWorkflowGetTextJSONAndPinnedData(t *testing.T) {
	f := workflowFixture(t, cliWorkflow)
	got := f.run("workflow", "get", "wf/one?x=1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Workflow:", "Invoice sync", "Published:", "Webhook", "n8n-nodes-base.webhook"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	if req := f.lastRequest(); req.URL.EscapedPath() != n8n.BasePath+"/workflows/wf%2Fone%3Fx=1" {
		t.Errorf("path = %q", req.URL.EscapedPath())
	}

	got = f.run("workflow", "get", "wf-1", "--exclude-pinned-data", "--output", "json")
	if f.lastRequest().URL.Query().Get("excludePinnedData") != "true" {
		t.Errorf("query = %q, want excludePinnedData", f.lastRequest().URL.RawQuery)
	}
	var workflow n8n.Workflow
	if err := json.Unmarshal([]byte(got.stdout), &workflow); err != nil || workflow.NodeCount() != 1 {
		t.Errorf("stdout = %q, want the workflow with its nodes (error: %v)", got.stdout, err)
	}
}

func TestWorkflowCreateFromInputDropsReadOnlyFields(t *testing.T) {
	f := workflowFixture(t, cliWorkflow)
	path := writeWorkflowFile(t, cliWorkflow)
	got := f.run("workflow", "create", "--input", path, "--project-id", "pr-2", "--parent-folder-id", "fo-1")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	req := f.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.WorkflowsPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(f.lastBody()), &body); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	for _, field := range []string{"id", "active", "versionId", "createdAt", "updatedAt", "tags", "description", "triggerCount"} {
		if _, ok := body[field]; ok {
			t.Errorf("request body carries the read-only field %q, which the API answers with 400", field)
		}
	}
	for _, field := range []string{"name", "nodes", "connections", "settings"} {
		if _, ok := body[field]; !ok {
			t.Errorf("request body lost the writable field %q", field)
		}
	}
	assertCLIJSON(t, `"pr-2"`, string(body["projectId"]))
	assertCLIJSON(t, `"fo-1"`, string(body["parentFolderId"]))
	// The user has to learn what was not sent.
	if !strings.Contains(got.stderr, "Not sent") || !strings.Contains(got.stderr, "id") {
		t.Errorf("stderr = %q, want the dropped fields reported", got.stderr)
	}
	if !strings.Contains(got.stdout, "Created:") {
		t.Errorf("stdout = %q, want the created workflow", got.stdout)
	}
}

func TestWorkflowCreateFromStdinAndName(t *testing.T) {
	t.Run("stdin", func(t *testing.T) {
		f := workflowFixture(t, cliWorkflow)
		f.stdin = `{"name":"From stdin","nodes":[],"connections":{},"settings":{}}`
		got := f.run("workflow", "create", "--input", "-")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"name":"From stdin","nodes":[],"connections":{},"settings":{}}`, f.lastBody())
	})

	t.Run("empty workflow from a name", func(t *testing.T) {
		f := workflowFixture(t, cliWorkflow)
		got := f.run("workflow", "create", "--name", "Scratch", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"name":"Scratch","nodes":[],"connections":{},"settings":{"executionOrder":"v1"}}`, f.lastBody())
		var workflow n8n.Workflow
		if err := json.Unmarshal([]byte(got.stdout), &workflow); err != nil || workflow.ID != "wf-1" {
			t.Errorf("stdout = %q, want the created workflow (error: %v)", got.stdout, err)
		}
	})

	t.Run("neither input nor name", func(t *testing.T) {
		f := workflowFixture(t, cliWorkflow)
		before := f.requestCount()
		got := f.run("workflow", "create")
		if got.code != ExitError || !strings.Contains(got.stderr, "--input or --name is required") {
			t.Errorf("result = %+v", got)
		}
		if f.requestCount() != before {
			t.Error("an incomplete create reached the instance")
		}
	})
}

func TestWorkflowCreateRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "not JSON", content: "nope", want: "decode workflow JSON"},
		{name: "two documents", content: `{"name":"a","nodes":[],"connections":{},"settings":{}} {"name":"b"}`, want: "multiple documents"},
		{name: "empty document", content: `{}`, want: "empty"},
		{name: "missing required fields", content: `{"name":"a"}`, want: "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := workflowFixture(t, cliWorkflow)
			before := f.requestCount()
			got := f.run("workflow", "create", "--input", writeWorkflowFile(t, tt.content))
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("an invalid document reached the instance")
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		f := workflowFixture(t, cliWorkflow)
		got := f.run("workflow", "create", "--input", filepath.Join(t.TempDir(), "absent.json"))
		if got.code != ExitError || !strings.Contains(got.stderr, "open workflow input") {
			t.Errorf("result = %+v", got)
		}
	})
}

func TestWorkflowUpdateReplacesDefinition(t *testing.T) {
	t.Run("input is required", func(t *testing.T) {
		f := workflowFixture(t, cliWorkflow)
		before := f.requestCount()
		got := f.run("workflow", "update", "wf-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "--input is required") {
			t.Errorf("result = %+v", got)
		}
		if f.requestCount() != before {
			t.Error("an update without a document reached the instance")
		}
	})

	t.Run("filtered body and name override", func(t *testing.T) {
		f := workflowFixture(t, cliWorkflow)
		got := f.run("workflow", "update", "wf/one?x=1", "--input", writeWorkflowFile(t, cliWorkflow), "--name", "Renamed")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+"/workflows/wf%2Fone%3Fx=1" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		if req.URL.RawQuery != "" {
			t.Errorf("query = %q, want the server default for publishIfActive", req.URL.RawQuery)
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal([]byte(f.lastBody()), &body); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		if _, ok := body["id"]; ok {
			t.Error("request body carries the read-only id")
		}
		assertCLIJSON(t, `"Renamed"`, string(body["name"]))
	})

	t.Run("draft only", func(t *testing.T) {
		f := workflowFixture(t, cliWorkflow)
		got := f.run("workflow", "update", "wf-1", "--input", writeWorkflowFile(t, cliWorkflow), "--no-publish")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if q := f.lastRequest().URL.Query().Get("publishIfActive"); q != "false" {
			t.Errorf("publishIfActive = %q, want false", q)
		}
	})
}

// A 403 on update is a partial success: the draft is saved, the published
// version stays live. The message has to say so, because nothing else will.
func TestWorkflowUpdateExplainsPublishDenial(t *testing.T) {
	f := workflowFixture(t, cliWorkflow)
	f.status = http.StatusForbidden
	got := f.run("workflow", "update", "wf-1", "--input", writeWorkflowFile(t, cliWorkflow))
	if got.code != ExitError {
		t.Fatalf("exit = %d, want an error", got.code)
	}
	for _, want := range []string{"403", "draft", "stays live", "--no-publish", "workflow:activate"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, got.stderr)
		}
	}

	// With --no-publish there is no publication to deny, so the message is the
	// ordinary scope explanation instead.
	got = f.run("workflow", "update", "wf-1", "--input", writeWorkflowFile(t, cliWorkflow), "--no-publish")
	if strings.Contains(got.stderr, "draft") {
		t.Errorf("stderr = %q, want the plain 403 explanation", got.stderr)
	}
}

func TestWorkflowLifecycleActions(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		method string
		path   string
		label  string
	}{
		{name: "archive", args: []string{"workflow", "archive", "wf/one?x=1"},
			method: http.MethodPost, path: "/workflows/wf%2Fone%3Fx=1/archive", label: "Archived:"},
		{name: "unarchive", args: []string{"workflow", "unarchive", "wf-1"},
			method: http.MethodPost, path: "/workflows/wf-1/unarchive", label: "Unarchived:"},
		{name: "publish", args: []string{"workflow", "publish", "wf-1"},
			method: http.MethodPost, path: "/workflows/wf-1/publish", label: "Published:"},
		{name: "unpublish", args: []string{"workflow", "unpublish", "wf-1", "--yes"},
			method: http.MethodPost, path: "/workflows/wf-1/unpublish", label: "Unpublished:"},
		{name: "delete", args: []string{"workflow", "delete", "wf-1", "--yes"},
			method: http.MethodDelete, path: "/workflows/wf-1", label: "Deleted:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := workflowFixture(t, cliWorkflow)
			f.interactive = false
			got := f.run(tt.args...)
			if got.code != ExitSuccess {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			req := f.lastRequest()
			if req.Method != tt.method || req.URL.EscapedPath() != n8n.BasePath+tt.path {
				t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.EscapedPath(), tt.method, tt.path)
			}
			if !strings.Contains(got.stdout, tt.label) {
				t.Errorf("stdout = %q, want %q", got.stdout, tt.label)
			}
		})
	}
}

func TestWorkflowPublishOptionsAndConflict(t *testing.T) {
	t.Run("version and labels", func(t *testing.T) {
		f := workflowFixture(t, cliWorkflow)
		got := f.run("workflow", "publish", "wf-1", "--version-id", "ver-2", "--name", "Release", "--description", "retry branch")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"versionId":"ver-2","name":"Release","description":"retry branch"}`, f.lastBody())
	})

	t.Run("conflict explains the block", func(t *testing.T) {
		f := workflowFixture(t, cliWorkflow)
		f.status = http.StatusConflict
		got := f.run("workflow", "publish", "wf-1")
		if got.code != ExitError {
			t.Fatalf("exit = %d, want an error", got.code)
		}
		for _, want := range []string{"409", "review", "webhook path"} {
			if !strings.Contains(got.stderr, want) {
				t.Errorf("stderr missing %q:\n%s", want, got.stderr)
			}
		}
	})
}

func TestWorkflowDestructiveCommandsConfirm(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		wanted []string
	}{
		{name: "delete", args: []string{"workflow", "delete", "wf-1"},
			wanted: []string{"Permanently delete", "cannot be undone", "archive"}},
		{name: "unpublish", args: []string{"workflow", "unpublish", "wf-1"},
			wanted: []string{"schedules stop running", "webhook"}},
		{name: "transfer", args: []string{"workflow", "transfer", "wf-1", "--destination-project-id", "pr-2"},
			wanted: []string{"Move workflow", "credentials"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := workflowFixture(t, cliWorkflow)
			before := f.requestCount()
			f.stdin = "no\n"
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
				t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
			}
			for _, want := range tt.wanted {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr missing %q:\n%s", want, got.stderr)
				}
			}

			f.interactive = false
			got = f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
				t.Errorf("non-interactive result = %+v", got)
			}
		})
	}
}

func TestWorkflowTransfer(t *testing.T) {
	t.Run("destination is required", func(t *testing.T) {
		f := workflowFixture(t, "")
		before := f.requestCount()
		got := f.run("workflow", "transfer", "wf-1", "--yes")
		if got.code != ExitError || !strings.Contains(got.stderr, "destination project ID is required") {
			t.Errorf("result = %+v", got)
		}
		if f.requestCount() != before {
			t.Error("a transfer without a destination reached the instance")
		}
	})

	t.Run("transferred", func(t *testing.T) {
		f := workflowFixture(t, "")
		f.status = http.StatusNoContent
		f.interactive = false
		got := f.run("workflow", "transfer", "wf/one?x=1", "--destination-project-id", "pr-2", "--yes", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPut || req.URL.EscapedPath() != n8n.BasePath+"/workflows/wf%2Fone%3Fx=1/transfer" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"destinationProjectId":"pr-2"}`, f.lastBody())
		var result workflowTransferResult
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Action != "transferred" || result.DestinationProjectID != "pr-2" {
			t.Errorf("stdout = %q, want a transfer acknowledgement (error: %v)", got.stdout, err)
		}
	})
}

func TestWorkflowHistoryAndVersion(t *testing.T) {
	const historyPage = `{"data":[{"versionId":"ver-1","workflowId":"wf-1","authors":"Ada Lovelace","name":null,"description":null,"createdAt":"2026-09-16T20:03:43.517Z","updatedAt":"2026-09-16T20:03:43.517Z"}],"nextCursor":"next"}`
	const versionBody = `{"versionId":"ver-1","workflowId":"wf-1","authors":"Ada Lovelace","name":null,"description":null,"nodes":[{"id":"n-1","name":"Webhook","type":"n8n-nodes-base.webhook"}],"connections":{}}`

	t.Run("history", func(t *testing.T) {
		f := workflowFixture(t, historyPage)
		got := f.run("workflow", "history", "wf/one?x=1", "--limit", "2")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.URL.EscapedPath() != n8n.BasePath+"/workflows/wf%2Fone%3Fx=1/history" || req.URL.Query().Get("limit") != "2" {
			t.Errorf("request = %s?%s", req.URL.EscapedPath(), req.URL.RawQuery)
		}
		for _, want := range []string{"ver-1", "Ada Lovelace", "Next cursor:"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("stdout missing %q:\n%s", want, got.stdout)
			}
		}
	})

	t.Run("empty history says so", func(t *testing.T) {
		f := workflowFixture(t, `{"data":[],"nextCursor":null}`)
		got := f.run("workflow", "history", "wf-1")
		if got.code != ExitSuccess || !strings.Contains(got.stderr, "no saved version history") {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("version get", func(t *testing.T) {
		f := workflowFixture(t, versionBody)
		got := f.run("workflow", "version", "get", "wf-1", "ver/two?y=2", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if want := n8n.BasePath + "/workflows/wf-1/versions/ver%2Ftwo%3Fy=2"; f.lastRequest().URL.EscapedPath() != want {
			t.Errorf("path = %q, want %q", f.lastRequest().URL.EscapedPath(), want)
		}
		var version n8n.WorkflowVersion
		if err := json.Unmarshal([]byte(got.stdout), &version); err != nil || version.VersionID != "ver-1" {
			t.Errorf("stdout = %q, want the stored version (error: %v)", got.stdout, err)
		}
	})

	t.Run("version get through the deprecated route", func(t *testing.T) {
		f := workflowFixture(t, versionBody)
		got := f.run("workflow", "version", "get-legacy", "wf-1", "ver-1")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if want := n8n.BasePath + "/workflows/wf-1/ver-1"; f.lastRequest().URL.EscapedPath() != want {
			t.Errorf("path = %q, want %q", f.lastRequest().URL.EscapedPath(), want)
		}
		if !strings.Contains(got.stderr, "deprecated") {
			t.Errorf("stderr = %q, want the deprecation notice", got.stderr)
		}
		if !strings.Contains(got.stdout, "Webhook") {
			t.Errorf("stdout = %q, want the version's nodes", got.stdout)
		}
	})

	t.Run("unknown version", func(t *testing.T) {
		f := workflowFixture(t, versionBody)
		f.status = http.StatusNotFound
		got := f.run("workflow", "version", "get", "wf-1", "ver-9")
		if got.code != ExitError || !strings.Contains(got.stderr, "n8n workflow history wf-1") {
			t.Errorf("result = %+v", got)
		}
	})
}

func TestWorkflowDeprecatedAliases(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		path   string
		notice string
	}{
		{name: "activate", args: []string{"workflow", "activate", "wf-1"},
			path: "/workflows/wf-1/activate", notice: "use 'n8n workflow publish'"},
		{name: "deactivate", args: []string{"workflow", "deactivate", "wf-1", "--yes"},
			path: "/workflows/wf-1/deactivate", notice: "use 'n8n workflow unpublish'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := workflowFixture(t, cliWorkflow)
			f.interactive = false
			got := f.run(tt.args...)
			if got.code != ExitSuccess {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			if want := n8n.BasePath + tt.path; f.lastRequest().URL.EscapedPath() != want {
				t.Errorf("path = %q, want %q", f.lastRequest().URL.EscapedPath(), want)
			}
			if !strings.Contains(got.stderr, tt.notice) {
				t.Errorf("stderr = %q, want %q", got.stderr, tt.notice)
			}
		})
	}
}

func TestWorkflowTags(t *testing.T) {
	const tagsBody = `[{"id":"tag-1","name":"production"}]`

	t.Run("list", func(t *testing.T) {
		f := workflowFixture(t, tagsBody)
		got := f.run("workflow", "tag", "list", "wf-1")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if want := n8n.BasePath + "/workflows/wf-1/tags"; f.lastRequest().URL.EscapedPath() != want {
			t.Errorf("path = %q, want %q", f.lastRequest().URL.EscapedPath(), want)
		}
		if !strings.Contains(got.stdout, "production") {
			t.Errorf("stdout = %q, want the tag", got.stdout)
		}
	})

	t.Run("set replaces", func(t *testing.T) {
		f := workflowFixture(t, tagsBody)
		got := f.run("workflow", "tag", "set", "wf-1", "--tag-id", "tag-1", "--tag-id", "tag-2", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", req.Method)
		}
		assertCLIJSON(t, `[{"id":"tag-1"},{"id":"tag-2"}]`, f.lastBody())
		var tags []n8n.Tag
		if err := json.Unmarshal([]byte(got.stdout), &tags); err != nil || len(tags) != 1 {
			t.Errorf("stdout = %q, want the resulting tags (error: %v)", got.stdout, err)
		}
	})

	t.Run("clear", func(t *testing.T) {
		f := workflowFixture(t, `[]`)
		got := f.run("workflow", "tag", "set", "wf-1", "--clear")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `[]`, f.lastBody())
		if !strings.Contains(got.stderr, "carries no tags") {
			t.Errorf("stderr = %q, want the empty-set note", got.stderr)
		}
	})

	t.Run("flags are mutually exclusive", func(t *testing.T) {
		f := workflowFixture(t, tagsBody)
		before := f.requestCount()
		got := f.run("workflow", "tag", "set", "wf-1", "--clear", "--tag-id", "tag-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "--clear cannot be combined") {
			t.Errorf("result = %+v", got)
		}
		got = f.run("workflow", "tag", "set", "wf-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "--tag-id or --clear is required") {
			t.Errorf("result = %+v", got)
		}
		if f.requestCount() != before {
			t.Error("an invalid tag set reached the instance")
		}
	})

	t.Run("unknown tag", func(t *testing.T) {
		f := workflowFixture(t, tagsBody)
		f.status = http.StatusNotFound
		got := f.run("workflow", "tag", "set", "wf-1", "--tag-id", "tag-9")
		if got.code != ExitError || !strings.Contains(got.stderr, "n8n tag list") {
			t.Errorf("result = %+v", got)
		}
	})
}

func TestWorkflowArgumentValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "blank get", args: []string{"workflow", "get", " "}, want: "workflow ID is required"},
		{name: "padded archive", args: []string{"workflow", "archive", " wf-1"}, want: "whitespace"},
		{name: "blank version", args: []string{"workflow", "version", "get", "wf-1", " "}, want: "version ID is required"},
		{name: "unknown output", args: []string{"workflow", "list", "--output", "yaml"}, want: "unknown output format"},
		{name: "negative limit", args: []string{"workflow", "list", "--limit", "-1"}, want: "limit"},
		{name: "blank tag list", args: []string{"workflow", "tag", "list", " "}, want: "workflow ID is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := workflowFixture(t, cliWorkflow)
			before := f.requestCount()
			f.interactive = false
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("an invalid invocation reached the instance")
			}
		})
	}
}

func TestWorkflowAPIErrorsExplainTheFix(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		want   []string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, args: []string{"workflow", "list"},
			want: []string{"401", "n8n auth login"}},
		{name: "forbidden", status: http.StatusForbidden, args: []string{"workflow", "publish", "wf-1"},
			want: []string{"403", "workflow:activate", "n8n discover --resource workflow"}},
		{name: "not found", status: http.StatusNotFound, args: []string{"workflow", "get", "wf-9"},
			want: []string{"404", "n8n workflow list"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := workflowFixture(t, cliWorkflow)
			f.status = tt.status
			f.interactive = false
			got := f.run(tt.args...)
			if got.code != ExitError {
				t.Fatalf("exit = %d, want an error", got.code)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr missing %q:\n%s", want, got.stderr)
				}
			}
		})
	}
}
