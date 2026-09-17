package cli

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliFolder = `{
  "id":"fo-1",
  "name":"Invoices",
  "parentFolderId":null,
  "createdAt":"2026-09-17T19:11:28.303Z",
  "updatedAt":"2026-09-17T19:11:28.303Z",
  "homeProject":{"id":"pr-1","name":"Personal","type":"personal"},
  "tags":[],
  "workflowCount":2,
  "subFolderCount":1
}`

const cliFolderDetail = `{
  "id":"fo-1",
  "name":"Invoices",
  "parentFolderId":"fo-0",
  "createdAt":"2026-09-17T19:11:28.303Z",
  "updatedAt":"2026-09-17T19:11:28.303Z",
  "totalSubFolders":3,
  "totalWorkflows":4
}`

func folderFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestFolderListTextJSONAndQuery(t *testing.T) {
	f := folderFixture(t, `{"count":7,"data":[`+cliFolder+`]}`)
	got := f.run("folder", "list", "pr/one?x=1", "--take", "1", "--skip", "2", "--sort-by", "name:asc")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Matching folders:", "7", "fo-1", "Invoices", "Next page:", "--skip 3"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	query := req.URL.Query()
	if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+"/projects/pr%2Fone%3Fx=1/folders" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	if query.Get("take") != "1" || query.Get("skip") != "2" || query.Get("sortBy") != "name:asc" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}

	got = f.run("folder", "list", "pr-1", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("JSON exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.FolderPage
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || page.Count != 7 || len(page.Data) != 1 || page.Data[0].ID != "fo-1" {
		t.Errorf("stdout = %q, want a folder page (error: %v)", got.stdout, err)
	}
}

func TestFolderListFilterFlagsAndJSONFilter(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "single filter flags",
			args: []string{"folder", "list", "pr-1", "--parent-folder-id", "fo-0", "--name", "Invoices", "--tag", "finance", "--tag", "paid", "--exclude-folder-id", "fo-9"},
			want: `{"parentFolderId":"fo-0","name":"Invoices","tags":["finance","paid"],"excludeFolderIdAndDescendants":"fo-9"}`,
		},
		{
			name: "filter document",
			args: []string{"folder", "list", "pr-1", "--filter", `{"name":"Invoices"}`},
			want: `{"name":"Invoices"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := folderFixture(t, `{"count":0,"data":[]}`)
			got := f.run(tt.args...)
			if got.code != ExitSuccess {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			assertCLIJSON(t, tt.want, f.lastRequest().URL.Query().Get("filter"))
		})
	}
}

func TestFolderListSelectEncodesJSONArray(t *testing.T) {
	f := folderFixture(t, `{"count":1,"data":[{"id":"fo-1","name":"Invoices"}]}`)
	got := f.run("folder", "list", "pr-1", "--select", "id", "--select", "name")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	assertCLIJSON(t, `["id","name"]`, f.lastRequest().URL.Query().Get("select"))
	// Fields the instance did not send must not be rendered as zero.
	row := regexp.MustCompile(`fo-1\s+Invoices\s+-\s+-\s+-`)
	if !row.MatchString(got.stdout) {
		t.Errorf("stdout = %q, want the unsent parent and counts rendered as dashes", got.stdout)
	}
}

func TestFolderListAllWalksOffsetPages(t *testing.T) {
	f := folderFixture(t, "")
	f.bodyFunc = func(page int) string {
		if page == 0 {
			return `{"count":3,"data":[{"id":"fo-1","name":"One"},{"id":"fo-2","name":"Two"}]}`
		}
		return `{"count":3,"data":[{"id":"fo-3","name":"Three"}]}`
	}
	got := f.run("folder", "list", "pr-1", "--all", "--take", "2", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.FolderPage
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 3 || page.Count != 3 {
		t.Errorf("stdout = %q, want three collected folders (error: %v)", got.stdout, err)
	}
	f.mu.Lock()
	requests := append([]*http.Request(nil), f.requests...)
	f.mu.Unlock()
	first, second := requests[len(requests)-2], requests[len(requests)-1]
	if first.URL.Query().Get("skip") != "" || second.URL.Query().Get("skip") != "2" {
		t.Errorf("skips = %q then %q", first.URL.RawQuery, second.URL.RawQuery)
	}
	if second.URL.Query().Get("take") != "2" {
		t.Errorf("page size not preserved: %q", second.URL.RawQuery)
	}
}

func TestFolderListEmptyExplainsNextStep(t *testing.T) {
	f := folderFixture(t, `{"count":0,"data":[]}`)
	got := f.run("folder", "list", "pr-1")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "n8n folder create") {
		t.Errorf("result = %+v, want a create hint on stderr", got)
	}
}

func TestFolderCreateGetAndUpdate(t *testing.T) {
	t.Run("create at project root", func(t *testing.T) {
		f := folderFixture(t, cliFolder)
		got := f.run("folder", "create", "personal", "Invoices", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+"/projects/personal/folders" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"name":"Invoices"}`, f.lastBody())
		var folder n8n.Folder
		if err := json.Unmarshal([]byte(got.stdout), &folder); err != nil || folder.ID != "fo-1" {
			t.Errorf("stdout = %q, want the created folder (error: %v)", got.stdout, err)
		}
	})

	t.Run("create inside a folder", func(t *testing.T) {
		f := folderFixture(t, cliFolder)
		got := f.run("folder", "create", "pr/one?x=1", "Invoices", "--parent-folder-id", "fo-0")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"name":"Invoices","parentFolderId":"fo-0"}`, f.lastBody())
		if !strings.Contains(got.stdout, "Created:") || !strings.Contains(got.stdout, "Invoices") {
			t.Errorf("stdout = %q, want a create acknowledgement", got.stdout)
		}
	})

	t.Run("get reports recursive totals", func(t *testing.T) {
		f := folderFixture(t, cliFolderDetail)
		got := f.run("folder", "get", "pr/one?x=1", "fo/two?y=2")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodGet || req.URL.EscapedPath() != n8n.BasePath+"/projects/pr%2Fone%3Fx=1/folders/fo%2Ftwo%3Fy=2" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		for _, want := range []string{"Sub-folders:", "3", "Workflows:", "4", "fo-0"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("stdout missing %q:\n%s", want, got.stdout)
			}
		}

		got = f.run("folder", "get", "pr-1", "fo-1", "--output", "json")
		var detail n8n.FolderDetail
		if err := json.Unmarshal([]byte(got.stdout), &detail); err != nil || detail.TotalWorkflows != 4 {
			t.Errorf("stdout = %q, want the folder detail (error: %v)", got.stdout, err)
		}
	})

	t.Run("update renames and moves", func(t *testing.T) {
		f := folderFixture(t, cliFolder)
		got := f.run("folder", "update", "pr/one?x=1", "fo/two?y=2", "--name", "Paid invoices", "--parent-folder-id", "fo-9", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPatch || req.URL.EscapedPath() != n8n.BasePath+"/projects/pr%2Fone%3Fx=1/folders/fo%2Ftwo%3Fy=2" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"name":"Paid invoices","parentFolderId":"fo-9"}`, f.lastBody())
		var folder n8n.Folder
		if err := json.Unmarshal([]byte(got.stdout), &folder); err != nil || folder.ID != "fo-1" {
			t.Errorf("stdout = %q, want the updated folder (error: %v)", got.stdout, err)
		}
	})

	t.Run("update sends only the given field", func(t *testing.T) {
		f := folderFixture(t, cliFolder)
		if got := f.run("folder", "update", "pr-1", "fo-1", "--parent-folder-id", "fo-9"); got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"parentFolderId":"fo-9"}`, f.lastBody())
	})
}

func TestFolderDeleteConfirmationAndTransfer(t *testing.T) {
	t.Run("declined before any request", func(t *testing.T) {
		f := folderFixture(t, "")
		before := f.requestCount()
		f.stdin = "no\n"
		got := f.run("folder", "delete", "pr-1", "fo-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
			t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
		}
		// The warning has to say what happens to the contents.
		for _, want := range []string{"archived", "every folder underneath", "--transfer-to-folder-id"} {
			if !strings.Contains(got.stderr, want) {
				t.Errorf("stderr missing %q:\n%s", want, got.stderr)
			}
		}

		f.interactive = false
		got = f.run("folder", "delete", "pr-1", "fo-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
			t.Errorf("non-interactive result = %+v", got)
		}
	})

	t.Run("transfer target changes the warning", func(t *testing.T) {
		f := folderFixture(t, "")
		f.stdin = "no\n"
		got := f.run("folder", "delete", "pr-1", "fo-1", "--transfer-to-folder-id", "fo-2")
		if got.code != ExitError {
			t.Fatalf("exit = %d, want the declined confirmation", got.code)
		}
		if !strings.Contains(got.stderr, "moved into folder") || strings.Contains(got.stderr, "archived") {
			t.Errorf("stderr = %q, want the transfer wording without the archive warning", got.stderr)
		}
	})

	t.Run("confirmed without a transfer target", func(t *testing.T) {
		f := folderFixture(t, "")
		f.interactive = false
		got := f.run("folder", "delete", "pr/one?x=1", "fo/two?y=2", "--yes", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodDelete || req.URL.EscapedPath() != n8n.BasePath+"/projects/pr%2Fone%3Fx=1/folders/fo%2Ftwo%3Fy=2" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		if req.URL.RawQuery != "" {
			t.Errorf("query = %q, want no transfer target", req.URL.RawQuery)
		}
		var result folderMutation
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Action != "deleted" || result.TransferToFolderID != "" {
			t.Errorf("stdout = %q, want a delete acknowledgement (error: %v)", got.stdout, err)
		}
	})

	t.Run("confirmed with a transfer target", func(t *testing.T) {
		f := folderFixture(t, "")
		f.interactive = false
		got := f.run("folder", "delete", "pr-1", "fo-1", "--transfer-to-folder-id", "fo/three?z=3", "--yes")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if got := req.URL.Query().Get("transferToFolderId"); got != "fo/three?z=3" {
			t.Errorf("transfer target = %q", got)
		}
		if !strings.Contains(got.stdout, "Contents moved to:") {
			t.Errorf("stdout = %q, want the transfer target echoed", got.stdout)
		}
	})
}

func TestFolderValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "blank project", args: []string{"folder", "list", " "}, want: "project ID is required"},
		{name: "unknown output", args: []string{"folder", "list", "pr-1", "--output", "yaml"}, want: "unknown output format"},
		{name: "negative skip", args: []string{"folder", "list", "pr-1", "--skip", "-1"}, want: "skip must not be negative"},
		{name: "negative take", args: []string{"folder", "list", "pr-1", "--take", "-1"}, want: "take must not be negative"},
		{name: "unknown sort order", args: []string{"folder", "list", "pr-1", "--sort-by", "name"}, want: "unknown sort order"},
		{name: "unknown select field", args: []string{"folder", "list", "pr-1", "--select", "nope"}, want: "unknown select field"},
		{name: "filter and single flags", args: []string{"folder", "list", "pr-1", "--filter", `{"name":"Invoices"}`, "--name", "Invoices"}, want: "cannot be combined"},
		{name: "filter unknown field", args: []string{"folder", "list", "pr-1", "--filter", `{"nope":1}`}, want: "unknown field"},
		{name: "filter not JSON", args: []string{"folder", "list", "pr-1", "--filter", "name=Invoices"}, want: "decode --filter JSON"},
		{name: "filter multiple documents", args: []string{"folder", "list", "pr-1", "--filter", `{"name":"a"} {"name":"b"}`}, want: "multiple documents"},
		{name: "filter narrows nothing", args: []string{"folder", "list", "pr-1", "--filter", `{}`}, want: "narrows nothing"},
		{name: "blank create name", args: []string{"folder", "create", "pr-1", " "}, want: "folder name is required"},
		{name: "padded create parent", args: []string{"folder", "create", "pr-1", "Invoices", "--parent-folder-id", "fo-1 "}, want: "whitespace"},
		{name: "blank get folder", args: []string{"folder", "get", "pr-1", " "}, want: "folder ID is required"},
		{name: "update without fields", args: []string{"folder", "update", "pr-1", "fo-1"}, want: "--name or --parent-folder-id is required"},
		{name: "padded update name", args: []string{"folder", "update", "pr-1", "fo-1", "--name", " Invoices"}, want: "whitespace"},
		{name: "blank delete folder", args: []string{"folder", "delete", "pr-1", " ", "--yes"}, want: "folder ID is required"},
		{name: "padded transfer target", args: []string{"folder", "delete", "pr-1", "fo-1", "--transfer-to-folder-id", " fo-2", "--yes"}, want: "whitespace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := folderFixture(t, `{"count":0,"data":[]}`)
			f.interactive = false
			before := f.requestCount()
			got := f.run(tt.args...)
			if got.code != ExitError || !strings.Contains(got.stderr, tt.want) {
				t.Errorf("result = %+v, want error containing %q", got, tt.want)
			}
			if f.requestCount() != before {
				t.Error("invalid invocation reached instance")
			}
		})
	}
}

func TestFolderScopeAndNotFoundErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		want   []string
	}{
		{name: "list unauthorized", status: http.StatusUnauthorized, args: []string{"folder", "list", "pr-1"}, want: []string{"401", "auth login"}},
		{name: "list forbidden", status: http.StatusForbidden, args: []string{"folder", "list", "pr-1"}, want: []string{"403", "folder:list", "discover --resource folders"}},
		{name: "create forbidden", status: http.StatusForbidden, args: []string{"folder", "create", "pr-1", "Invoices"}, want: []string{"403", "folder:create"}},
		{name: "delete forbidden", status: http.StatusForbidden, args: []string{"folder", "delete", "pr-1", "fo-1", "--yes"}, want: []string{"403", "folder:delete"}},
		{name: "get not found", status: http.StatusNotFound, args: []string{"folder", "get", "pr-1", "fo-1"}, want: []string{"404", "n8n folder list", "personal"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := folderFixture(t, "")
			f.status = tt.status
			f.interactive = false
			got := f.run(tt.args...)
			if got.code != ExitError || got.stdout != "" {
				t.Errorf("result = %+v, want failure with empty stdout", got)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want %q", got.stderr, want)
				}
			}
		})
	}
}
