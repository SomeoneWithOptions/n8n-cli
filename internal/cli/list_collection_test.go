package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

// collectedJSON is the --all JSON shape the tests decode.
type collectedJSON struct {
	Count      *int            `json:"count"`
	Data       json.RawMessage `json:"data"`
	NextCursor *string         `json:"nextCursor"`
	Collection *struct {
		Truncated bool `json:"truncated"`
		Limit     int  `json:"limit"`
		NextSkip  *int `json:"nextSkip"`
	} `json:"collection"`
}

func decodeCollected(t *testing.T, stdout string) (collectedJSON, int) {
	t.Helper()
	var got collectedJSON
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%.300s", err, stdout)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(got.Data, &rows); err != nil || rows == nil {
		t.Fatalf("data = %.100s, want an array (error: %v)", got.Data, err)
	}
	return got, len(rows)
}

// tagPagesRoute serves total tags in pages of size, chained by the cursors
// "page-1", "page-2", ..., ignoring the requested limit like some servers do.
func tagPagesRoute(total, size int) func(*http.Request, int) (int, string) {
	return func(r *http.Request, _ int) (int, string) {
		index := 0
		if cursor := r.URL.Query().Get("cursor"); cursor != "" {
			n, err := strconv.Atoi(strings.TrimPrefix(cursor, "page-"))
			if err != nil {
				return http.StatusBadRequest, `{"message":"bad cursor"}`
			}
			index = n
		}
		var body strings.Builder
		body.WriteString(`{"data":[`)
		for i := index * size; i < total && i < (index+1)*size; i++ {
			if i > index*size {
				body.WriteByte(',')
			}
			fmt.Fprintf(&body, `{"id":"%d","name":"tag-%d"}`, i, i)
		}
		next := "null"
		if (index+1)*size < total {
			next = strconv.Quote(fmt.Sprintf("page-%d", index+1))
		}
		fmt.Fprintf(&body, `],"nextCursor":%s}`, next)
		return http.StatusOK, body.String()
	}
}

func TestListCollectionAtTheRealCap(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		size      int
		truncated bool
		cursor    string
		requests  int
		stderr    []string
	}{
		{name: "exactly the cap", total: n8n.DefaultCollectLimit, size: 250, requests: 40},
		{
			name: "one over the cap on a page boundary", total: n8n.DefaultCollectLimit + 1, size: 250,
			truncated: true, cursor: "page-40", requests: 40,
			stderr: []string{"Warning: collection stopped at the 10000-item limit", "--cursor page-40,"},
		},
		{
			name: "cap inside a page", total: n8n.DefaultCollectLimit + 1, size: 300,
			truncated: true, requests: 34,
			stderr: []string{"Warning: collection stopped at the 10000-item limit", "no cursor resumes"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tagFixture(t, "")
			f.route = tagPagesRoute(tt.total, tt.size)
			before := f.requestCount()
			got := f.run("tag", "list", "--all", "--limit", "250", "--output", "json")
			if got.code != ExitSuccess {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			if requests := f.requestCount() - before; requests != tt.requests {
				t.Errorf("requests = %d, want %d and no probe past the cap", requests, tt.requests)
			}
			page, rows := decodeCollected(t, got.stdout)
			if rows != n8n.DefaultCollectLimit {
				t.Errorf("rows = %d, want %d", rows, n8n.DefaultCollectLimit)
			}
			if page.Collection == nil || page.Collection.Truncated != tt.truncated || page.Collection.Limit != n8n.DefaultCollectLimit {
				t.Errorf("collection = %+v, want truncated %t at the 10000 limit", page.Collection, tt.truncated)
			}
			if page.NextCursor == nil || *page.NextCursor != tt.cursor {
				t.Errorf("nextCursor = %v, want %q", page.NextCursor, tt.cursor)
			}
			if !tt.truncated && got.stderr != "" {
				t.Errorf("stderr = %q, want no warning for an exhausted list", got.stderr)
			}
			for _, want := range tt.stderr {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want %q", got.stderr, want)
				}
			}
		})
	}
}

func TestListCollectionTextWarnsOnStderrOnly(t *testing.T) {
	f := tagFixture(t, "")
	f.route = tagPagesRoute(n8n.DefaultCollectLimit+1, 250)
	got := f.run("tag", "list", "--all", "--limit", "250")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if strings.Contains(got.stdout, "Warning") || !strings.Contains(got.stdout, "Next cursor:") {
		t.Errorf("stdout should keep the table and resumable cursor, without the warning:\n%.300s", got.stdout)
	}
	if !strings.Contains(got.stderr, "Warning: collection stopped") {
		t.Errorf("stderr = %q, want the cap warning", got.stderr)
	}
}

func TestListCollectionScriptViewsWarnOnce(t *testing.T) {
	tests := []struct {
		name   string
		size   int
		stderr []string
	}{
		{name: "cap on a page boundary", size: 250, stderr: []string{"Warning: collection stopped", "Continue with --cursor page-40,"}},
		{name: "cap inside a page", size: 300, stderr: []string{"Warning: collection stopped", "no cursor resumes"}},
	}
	for _, tt := range tests {
		for _, view := range []string{"--quiet", "--brief"} {
			t.Run(tt.name+" "+view, func(t *testing.T) {
				f := tagFixture(t, "")
				f.route = tagPagesRoute(n8n.DefaultCollectLimit+1, tt.size)
				got := f.run("tag", "list", "--all", "--limit", "250", view)
				if got.code != ExitSuccess {
					t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
				}
				if lines := strings.Count(got.stdout, "\n"); lines != n8n.DefaultCollectLimit {
					t.Errorf("stdout lines = %d, want %d rows and nothing else", lines, n8n.DefaultCollectLimit)
				}
				for _, want := range tt.stderr {
					if !strings.Contains(got.stderr, want) {
						t.Errorf("stderr = %q, want %q", got.stderr, want)
					}
				}
				if strings.Contains(got.stderr, "or use --all") || strings.Count(got.stderr, "--cursor") > 1 {
					t.Errorf("stderr = %q, want only the collection warning, not the single-page cursor hint", got.stderr)
				}
			})
		}
	}
}

func TestListCollectionScriptViewExhaustedIsSilent(t *testing.T) {
	f := tagFixture(t, "")
	f.route = tagPagesRoute(600, 250)
	got := f.run("tag", "list", "--all", "--quiet")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if lines := strings.Count(got.stdout, "\n"); lines != 600 {
		t.Errorf("stdout lines = %d, want 600", lines)
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want nothing for an exhausted list", got.stderr)
	}
}

func TestListCollectionKeepsSinglePageShape(t *testing.T) {
	f := tagFixture(t, `{"data":[],"nextCursor":null}`)
	got := f.run("tag", "list", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	if want := "{\n  \"data\": [],\n  \"nextCursor\": \"\"\n}\n"; got.stdout != want {
		t.Errorf("stdout = %q, want the unchanged page shape %q", got.stdout, want)
	}

	got = f.run("tag", "list", "--all", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	page, rows := decodeCollected(t, got.stdout)
	if rows != 0 || page.Collection == nil || page.Collection.Truncated || page.Collection.NextSkip != nil {
		t.Errorf("stdout = %s, want [] with a complete collection report", got.stdout)
	}
}

func TestListCollectionErrorAfterPagesPrintsNothing(t *testing.T) {
	f := tagFixture(t, "")
	pages := tagPagesRoute(10, 2)
	f.route = func(r *http.Request, n int) (int, string) {
		if n == 2 {
			return http.StatusForbidden, `{"message":"forbidden"}`
		}
		return pages(r, n)
	}
	got := f.run("tag", "list", "--all", "--output", "json")
	if got.code != ExitError || got.stdout != "" {
		t.Fatalf("result = %+v, want an error and no partial output", got)
	}
	if !strings.Contains(got.stderr, "tag:list") {
		t.Errorf("stderr = %q, want the resource remediation kept", got.stderr)
	}
}

// TestListCollectionMigratedCommandsReport runs every migrated cursor --all
// path once, so none can silently drop the collection report.
func TestListCollectionMigratedCommandsReport(t *testing.T) {
	for _, args := range [][]string{
		{"workflow", "list"},
		{"workflow", "history", "wf-1"},
		{"execution", "list"},
		{"execution", "list", "--status", "success"},
		{"tag", "list"},
		{"credential", "list"},
		{"project", "list"},
		{"project", "user", "list", "pr-1"},
		{"data-table", "list"},
		{"data-table", "row", "list", "dt-1"},
		{"variable", "list"},
		{"user", "list"},
		{"role-mapping", "list"},
		{"git-connection", "list"},
		{"promotion", "provider", "list"},
		{"promotion", "connection", "list"},
		{"evaluation", "list", "wf-1"},
		{"evaluation", "case", "list", "wf-1", "run-1"},
		{"ldap", "sync", "history"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f := tagFixture(t, "")
			f.route = func(r *http.Request, _ int) (int, string) {
				if r.URL.Query().Get("cursor") == "" {
					return http.StatusOK, `{"data":[{}],"nextCursor":"next"}`
				}
				return http.StatusOK, `{"data":[{}],"nextCursor":null}`
			}
			got := f.run(append(args, "--all", "--output", "json")...)
			if got.code != ExitSuccess {
				t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
			}
			page, rows := decodeCollected(t, got.stdout)
			if rows != 2 || page.Collection == nil || page.Collection.Truncated {
				t.Errorf("stdout = %.300s, want both pages and a complete collection report", got.stdout)
			}
		})
	}
}

func TestWritePageJSON(t *testing.T) {
	var out bytes.Buffer
	if err := writePageJSON(&out, n8n.Page[string]{Data: []string{}}, nil); err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"data\": [],\n  \"nextCursor\": \"\"\n}\n"; out.String() != want {
		t.Errorf("ordinary page = %q, want the API page unchanged %q", out.String(), want)
	}

	out.Reset()
	c := cursorCollection(true, "", 10)
	if err := writePageJSON(&out, n8n.Page[string]{}, c); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"data\": [],\n  \"nextCursor\": \"\",\n  \"collection\": {\n    \"truncated\": true,\n    \"limit\": 10\n  }\n}\n"
	if out.String() != want {
		t.Errorf("collection = %q, want %q", out.String(), want)
	}

	out.Reset()
	skip := 7
	folders := &listCollection{info: collectionInfo{Truncated: true, Limit: 10, NextSkip: &skip}, resume: resumeSkip, skip: skip}
	if err := writeFolderPageJSON(&out, n8n.FolderPage{Count: 12}, folders); err != nil {
		t.Fatal(err)
	}
	want = "{\n  \"count\": 12,\n  \"data\": [],\n  \"collection\": {\n    \"truncated\": true,\n    \"limit\": 10,\n    \"nextSkip\": 7\n  }\n}\n"
	if out.String() != want {
		t.Errorf("folder collection = %q, want %q", out.String(), want)
	}
}

func TestFinishCollection(t *testing.T) {
	boom := errors.New("boom")
	skip := 40
	tests := []struct {
		name string
		c    *listCollection
		err  error
		want []string
	}{
		{name: "single page", c: nil},
		{name: "complete", c: cursorCollection(false, "", 10)},
		{name: "render failed", c: cursorCollection(true, "next", 10), err: boom},
		{name: "resumable cursor", c: cursorCollection(true, "a\x1b[2Jb", 10), want: []string{"10-item limit", `--cursor a\u001b[2Jb,`}},
		{name: "clipped", c: cursorCollection(true, "", 10), want: []string{"no cursor resumes", "--limit and --cursor"}},
		{name: "folders", c: &listCollection{info: collectionInfo{Truncated: true, Limit: 10, NextSkip: &skip}, resume: resumeSkip, skip: skip}, want: []string{"--skip 40"}},
		{name: "merged statuses", c: &listCollection{info: collectionInfo{Truncated: true, Limit: 10}, resume: resumeMultiStatus}, want: []string{"10-item limit", "Repeating --all does not reach", "each status separately"}},
		{name: "merged page", c: &listCollection{info: collectionInfo{Truncated: true, Limit: 30}, resume: resumeMultiStatusPage}, want: []string{"More executions match these statuses"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errOut bytes.Buffer
			if err := finishCollection(&errOut, tt.c, tt.err); !errors.Is(err, tt.err) {
				t.Errorf("error = %v, want %v passed through", err, tt.err)
			}
			if len(tt.want) == 0 && errOut.Len() != 0 {
				t.Errorf("stderr = %q, want nothing", errOut.String())
			}
			for _, want := range tt.want {
				if !strings.Contains(errOut.String(), want) {
					t.Errorf("stderr = %q, want %q", errOut.String(), want)
				}
			}
			if strings.ContainsRune(errOut.String(), '\x1b') {
				t.Errorf("stderr = %q carries a raw control character", errOut.String())
			}
		})
	}
}
