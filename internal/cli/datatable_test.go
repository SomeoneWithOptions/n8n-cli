package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const cliDataTable = `{
  "id":"dt-1",
  "name":"customers",
  "projectId":"pr-1",
  "createdAt":"2026-09-17T19:31:31.471Z",
  "updatedAt":"2026-09-17T19:31:31.471Z",
  "sizeBytes":16384,
  "columns":[{"id":"co-1","dataTableId":"dt-1","name":"email","type":"string","index":0}]
}`

const cliDataTableRow = `{
  "id":1,
  "createdAt":"2026-09-17T19:31:42.152Z",
  "updatedAt":"2026-09-17T19:31:42.152Z",
  "email":"a@example.com",
  "age":30,
  "active":true,
  "seen":null
}`

func dataTableFixture(t *testing.T, body string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.login()
	f.body = body
	return f
}

func TestDataTableRequiresAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		authType n8n.AuthType
		envName  string
	}{
		{name: "bearer", authType: n8n.AuthBearer, envName: config.EnvBearerToken},
		{name: "cookie", authType: n8n.AuthCookie, envName: config.EnvAuthCookie},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.env[config.EnvURL] = f.server.URL
			f.env[tt.envName] = "not-an-api-key"
			got := f.run("data-table", "list")
			if got.code != ExitError {
				t.Fatalf("exit code = %d, want %d", got.code, ExitError)
			}
			for _, want := range []string{"API-key", string(tt.authType), config.EnvAPIKey} {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want %q", got.stderr, want)
				}
			}
			if f.requestCount() != 0 {
				t.Error("non-API-key command reached instance")
			}
		})
	}
}

func TestDataTableListTextJSONAndQuery(t *testing.T) {
	f := dataTableFixture(t, `{"data":[`+cliDataTable+`],"nextCursor":"next/one?x=1"}`)
	got := f.run("data-table", "list", "--limit", "50", "--cursor", "prev/one?x=1", "--name", "customers", "--sort-by", "size:desc")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Data tables:", "dt-1", "customers", "16384", "Next cursor:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	query := req.URL.Query()
	if req.Method != http.MethodGet || req.URL.Path != n8n.BasePath+n8n.DataTablesPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if query.Get("limit") != "50" || query.Get("cursor") != "prev/one?x=1" || query.Get("sortBy") != "size:desc" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}
	assertCLIJSON(t, `{"name":"customers"}`, query.Get("filter"))

	got = f.run("data-table", "list", "--output", "json")
	var page n8n.Page[n8n.DataTable]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 || page.Data[0].SizeBytes != 16384 {
		t.Errorf("stdout = %q, want a data table page (error: %v)", got.stdout, err)
	}
}

func TestDataTableListAllFollowsCursor(t *testing.T) {
	f := dataTableFixture(t, "")
	f.bodyFunc = func(page int) string {
		if page == 0 {
			return `{"data":[` + cliDataTable + `],"nextCursor":"next"}`
		}
		return `{"data":[{"id":"dt-2","name":"orders"}],"nextCursor":null}`
	}
	got := f.run("data-table", "list", "--all", "--limit", "1", "--name", "customers", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	var page n8n.Page[n8n.DataTable]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 2 {
		t.Errorf("stdout = %q, want both pages collected (error: %v)", got.stdout, err)
	}
	f.mu.Lock()
	requests := append([]*http.Request(nil), f.requests...)
	f.mu.Unlock()
	second := requests[len(requests)-1]
	if second.URL.Query().Get("cursor") != "next" {
		t.Errorf("second cursor = %q", second.URL.RawQuery)
	}
	// The filter has to survive paging, or later pages quietly widen.
	assertCLIJSON(t, `{"name":"customers"}`, second.URL.Query().Get("filter"))
}

func TestDataTableListEmptyExplainsNextStep(t *testing.T) {
	f := dataTableFixture(t, `{"data":[],"nextCursor":null}`)
	got := f.run("data-table", "list")
	if got.code != ExitSuccess || !strings.Contains(got.stderr, "n8n data-table create") {
		t.Errorf("result = %+v, want a create hint on stderr", got)
	}
}

func TestDataTableCreateGetUpdateAndDelete(t *testing.T) {
	t.Run("create sends columns in order", func(t *testing.T) {
		f := dataTableFixture(t, cliDataTable)
		got := f.run("data-table", "create", "customers",
			"--column", "email:string", "--column", "age:number", "--project-id", "pr-1", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.Path != n8n.BasePath+n8n.DataTablesPath {
			t.Errorf("request = %s %s", req.Method, req.URL.Path)
		}
		assertCLIJSON(t, `{"name":"customers","columns":[{"name":"email","type":"string"},{"name":"age","type":"number"}],"projectId":"pr-1"}`, f.lastBody())
		var table n8n.DataTable
		if err := json.Unmarshal([]byte(got.stdout), &table); err != nil || table.ID != "dt-1" {
			t.Errorf("stdout = %q, want the created table (error: %v)", got.stdout, err)
		}
	})

	t.Run("get shows columns", func(t *testing.T) {
		f := dataTableFixture(t, cliDataTable)
		got := f.run("data-table", "get", "dt/one?x=1")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if req := f.lastRequest(); req.URL.EscapedPath() != n8n.BasePath+"/data-tables/dt%2Fone%3Fx=1" {
			t.Errorf("path = %s", req.URL.EscapedPath())
		}
		for _, want := range []string{"customers", "Size bytes:", "INDEX", "email", "string"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("stdout missing %q:\n%s", want, got.stdout)
			}
		}
	})

	t.Run("update renames", func(t *testing.T) {
		f := dataTableFixture(t, cliDataTable)
		got := f.run("data-table", "update", "dt-1", "--name", "renamed")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		if req := f.lastRequest(); req.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", req.Method)
		}
		assertCLIJSON(t, `{"name":"renamed"}`, f.lastBody())
	})

	t.Run("delete confirmation", func(t *testing.T) {
		f := dataTableFixture(t, "")
		before := f.requestCount()
		f.stdin = "no\n"
		got := f.run("data-table", "delete", "dt-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
			t.Errorf("declined result = %+v, requests = %d want %d", got, f.requestCount(), before)
		}
		for _, want := range []string{"Every row and column", "workflows that use the table will fail"} {
			if !strings.Contains(got.stderr, want) {
				t.Errorf("stderr missing %q:\n%s", want, got.stderr)
			}
		}
		f.interactive = false
		got = f.run("data-table", "delete", "dt-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "pass --yes") || f.requestCount() != before {
			t.Errorf("non-interactive result = %+v", got)
		}
		got = f.run("data-table", "delete", "dt/one?x=1", "--yes", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodDelete || req.URL.EscapedPath() != n8n.BasePath+"/data-tables/dt%2Fone%3Fx=1" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		var result dataTableMutation
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Action != "deleted" {
			t.Errorf("stdout = %q, want a delete acknowledgement (error: %v)", got.stdout, err)
		}
	})
}

func TestDataTableRowListFiltersSortAndSearch(t *testing.T) {
	f := dataTableFixture(t, `{"data":[`+cliDataTableRow+`],"nextCursor":"next"}`)
	got := f.run("data-table", "row", "list", "dt/one?x=1",
		"--where", "status=active", "--where", "age=gte=30", "--match", "or",
		"--sort-by", "age:desc", "--search", "example", "--limit", "5")
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	for _, want := range []string{"Rows:", "ID", "EMAIL", "a@example.com", "30", "true", "Next cursor:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, got.stdout)
		}
	}
	req := f.lastRequest()
	if req.URL.EscapedPath() != n8n.BasePath+"/data-tables/dt%2Fone%3Fx=1/rows" {
		t.Errorf("path = %s", req.URL.EscapedPath())
	}
	query := req.URL.Query()
	if query.Get("sortBy") != "age:desc" || query.Get("search") != "example" || query.Get("limit") != "5" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}
	assertCLIJSON(t, `{"type":"or","filters":[
		{"columnName":"status","condition":"eq","value":"active"},
		{"columnName":"age","condition":"gte","value":30}]}`, query.Get("filter"))

	got = f.run("data-table", "row", "list", "dt-1", "--output", "json")
	var page n8n.Page[n8n.DataTableRow]
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || len(page.Data) != 1 {
		t.Fatalf("stdout = %q, want a row page (error: %v)", got.stdout, err)
	}
	// JSON output is the faithful copy: types and nulls survive it.
	if string(page.Data[0]["age"]) != "30" || string(page.Data[0]["seen"]) != "null" {
		t.Errorf("row = %v, want the original JSON values", page.Data[0])
	}
}

func TestDataTableRowListFilterDocument(t *testing.T) {
	f := dataTableFixture(t, `{"data":[],"nextCursor":null}`)
	filter := `{"type":"and","filters":[{"columnName":"status","condition":"eq","value":"active"}]}`
	got := f.run("data-table", "row", "list", "dt-1", "--filter", filter)
	if got.code != ExitSuccess {
		t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
	}
	assertCLIJSON(t, filter, f.lastRequest().URL.Query().Get("filter"))
}

func TestDataTableRowInsertFromFlagsAndInput(t *testing.T) {
	t.Run("set pairs type their values", func(t *testing.T) {
		f := dataTableFixture(t, `{"success":true,"insertedRows":1}`)
		got := f.run("data-table", "row", "insert", "dt-1",
			"--set", "email=a@example.com", "--set", "age=30", "--set", "active=true", "--set", "note=null")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"data":[{"email":"a@example.com","age":30,"active":true,"note":null}]}`, f.lastBody())
		if !strings.Contains(got.stdout, "Inserted:") || !strings.Contains(got.stdout, "1") {
			t.Errorf("stdout = %q, want the inserted count", got.stdout)
		}
	})

	t.Run("return all prints the rows", func(t *testing.T) {
		f := dataTableFixture(t, `[`+cliDataTableRow+`]`)
		got := f.run("data-table", "row", "insert", "dt-1", "--set", "email=a@example.com", "--return", "all")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"data":[{"email":"a@example.com"}],"returnType":"all"}`, f.lastBody())
		if !strings.Contains(got.stdout, "a@example.com") || !strings.Contains(got.stdout, "Row IDs:") {
			t.Errorf("stdout = %q, want the returned row and its ID", got.stdout)
		}
	})

	t.Run("stdin array", func(t *testing.T) {
		f := dataTableFixture(t, `[{"id":2}]`)
		f.stdin = `[{"email":"b@example.com","age":25}]`
		got := f.run("data-table", "row", "insert", "dt-1", "--input", "-", "--return", "id", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"data":[{"email":"b@example.com","age":25}],"returnType":"id"}`, f.lastBody())
		// The API decoder is shape-driven, so read the CLI's own output with a
		// plain struct rather than feeding it back through InsertRowsResult.
		var result struct {
			IDs []int64 `json:"ids"`
		}
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || len(result.IDs) != 1 || result.IDs[0] != 2 {
			t.Errorf("stdout = %q, want the inserted ID (error: %v)", got.stdout, err)
		}
	})

	t.Run("file with a single object", func(t *testing.T) {
		f := dataTableFixture(t, `{"count":1}`)
		path := filepath.Join(t.TempDir(), "row.json")
		if err := os.WriteFile(path, []byte(`{"email":"c@example.com"}`), 0o600); err != nil {
			t.Fatalf("write row input: %v", err)
		}
		if got := f.run("data-table", "row", "insert", "dt-1", "--input", path); got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"data":[{"email":"c@example.com"}]}`, f.lastBody())
	})
}

func TestDataTableRowUpdateUpsertAndDryRun(t *testing.T) {
	dryRunRows := `[{"id":1,"age":30,"dryRunState":"before"},{"id":1,"age":31,"dryRunState":"after"}]`

	t.Run("update sends filter and values", func(t *testing.T) {
		f := dataTableFixture(t, `true`)
		got := f.run("data-table", "row", "update", "dt-1", "--where", "status=pending", "--set", "status=done")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPatch || req.URL.EscapedPath() != n8n.BasePath+"/data-tables/dt-1/rows/update" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"filter":{"type":"and","filters":[{"columnName":"status","condition":"eq","value":"pending"}]},"data":{"status":"done"}}`, f.lastBody())
		if !strings.Contains(got.stdout, "Acknowledged:\ttrue") && !strings.Contains(got.stdout, "Acknowledged:  true") {
			t.Errorf("stdout = %q, want the acknowledgement", got.stdout)
		}
	})

	t.Run("dry run asks for the rows and says nothing changed", func(t *testing.T) {
		f := dataTableFixture(t, dryRunRows)
		got := f.run("data-table", "row", "update", "dt-1", "--where", "status=pending", "--set", "status=done", "--dry-run")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"filter":{"type":"and","filters":[{"columnName":"status","condition":"eq","value":"pending"}]},"data":{"status":"done"},"returnData":true,"dryRun":true}`, f.lastBody())
		for _, want := range []string{"dry run, nothing was changed", "DRYRUNSTATE", "before", "after"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("stdout missing %q:\n%s", want, got.stdout)
			}
		}
	})

	t.Run("upsert with a JSON body", func(t *testing.T) {
		f := dataTableFixture(t, cliDataTableRow)
		f.stdin = `{"name":"Jane","age":31}`
		got := f.run("data-table", "row", "upsert", "dt-1", "--where", "email=a@example.com", "--input", "-", "--return-data", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPost || req.URL.EscapedPath() != n8n.BasePath+"/data-tables/dt-1/rows/upsert" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"filter":{"type":"and","filters":[{"columnName":"email","condition":"eq","value":"a@example.com"}]},"data":{"name":"Jane","age":31},"returnData":true}`, f.lastBody())
		var result n8n.RowMutationResult
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || len(result.Rows) != 1 {
			t.Errorf("stdout = %q, want the upserted row (error: %v)", got.stdout, err)
		}
	})
}

func TestDataTableRowClearAndDelete(t *testing.T) {
	t.Run("clear confirmation", func(t *testing.T) {
		f := dataTableFixture(t, `{"deletedCount":3}`)
		before := f.requestCount()
		f.stdin = "no\n"
		got := f.run("data-table", "row", "clear", "dt-1")
		if got.code != ExitError || !strings.Contains(got.stderr, "aborted") || f.requestCount() != before {
			t.Errorf("declined result = %+v", got)
		}
		if !strings.Contains(got.stderr, "table and its columns stay") {
			t.Errorf("stderr = %q, want the data-loss wording", got.stderr)
		}
		got = f.run("data-table", "row", "clear", "dt-1", "--yes")
		if got.code != ExitSuccess {
			t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodDelete || req.URL.EscapedPath() != n8n.BasePath+"/data-tables/dt-1/rows/clear" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		if !strings.Contains(got.stdout, "Deleted rows:") || !strings.Contains(got.stdout, "3") {
			t.Errorf("stdout = %q, want the deleted count", got.stdout)
		}
	})

	t.Run("delete confirmation names the filter", func(t *testing.T) {
		f := dataTableFixture(t, `true`)
		before := f.requestCount()
		f.stdin = "no\n"
		got := f.run("data-table", "row", "delete", "dt-1", "--where", "status=archived")
		if got.code != ExitError || f.requestCount() != before {
			t.Errorf("declined result = %+v", got)
		}
		for _, want := range []string{`status eq "archived"`, "--dry-run"} {
			if !strings.Contains(got.stderr, want) {
				t.Errorf("stderr missing %q:\n%s", want, got.stderr)
			}
		}
	})

	t.Run("dry run needs no confirmation", func(t *testing.T) {
		f := dataTableFixture(t, `[{"id":1,"dryRunState":"before"}]`)
		f.interactive = false
		got := f.run("data-table", "row", "delete", "dt-1", "--where", "status=archived", "--dry-run")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		query := f.lastRequest().URL.Query()
		if query.Get("dryRun") != "true" || query.Get("returnData") != "true" {
			t.Errorf("query = %q, want a dry run asking for rows", f.lastRequest().URL.RawQuery)
		}
		assertCLIJSON(t, `{"type":"and","filters":[{"columnName":"status","condition":"eq","value":"archived"}]}`, query.Get("filter"))
		if !strings.Contains(got.stdout, "dry run, nothing was changed") {
			t.Errorf("stdout = %q, want the dry-run wording", got.stdout)
		}
	})

	t.Run("confirmed delete", func(t *testing.T) {
		f := dataTableFixture(t, `true`)
		f.interactive = false
		got := f.run("data-table", "row", "delete", "dt/one?x=1", "--where", "age=lt=18", "--yes", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodDelete || req.URL.EscapedPath() != n8n.BasePath+"/data-tables/dt%2Fone%3Fx=1/rows/delete" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"type":"and","filters":[{"columnName":"age","condition":"lt","value":18}]}`, req.URL.Query().Get("filter"))
		var result n8n.RowMutationResult
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || !result.Acknowledged {
			t.Errorf("stdout = %q, want the acknowledgement (error: %v)", got.stdout, err)
		}
	})
}

func TestDataTableColumnCommands(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		f := dataTableFixture(t, `[{"id":"co-1","dataTableId":"dt-1","name":"email","type":"string","index":0}]`)
		got := f.run("data-table", "column", "list", "dt-1")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		for _, want := range []string{"Columns:", "co-1", "email", "string"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("stdout missing %q:\n%s", want, got.stdout)
			}
		}
		if req := f.lastRequest(); req.URL.EscapedPath() != n8n.BasePath+"/data-tables/dt-1/columns" {
			t.Errorf("path = %s", req.URL.EscapedPath())
		}
	})

	t.Run("create appends without an index", func(t *testing.T) {
		f := dataTableFixture(t, `{"id":"co-2","dataTableId":"dt-1","name":"status","type":"string","index":1}`)
		got := f.run("data-table", "column", "create", "dt-1", "status", "string")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"name":"status","type":"string"}`, f.lastBody())
		if !strings.Contains(got.stdout, "Created:") {
			t.Errorf("stdout = %q", got.stdout)
		}
	})

	t.Run("create at index zero", func(t *testing.T) {
		f := dataTableFixture(t, `{"id":"co-2","name":"status","type":"string","index":0}`)
		if got := f.run("data-table", "column", "create", "dt-1", "status", "string", "--index", "0"); got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		assertCLIJSON(t, `{"name":"status","type":"string","index":0}`, f.lastBody())
	})

	t.Run("update renames and reorders", func(t *testing.T) {
		f := dataTableFixture(t, `{"id":"co-2","name":"state","type":"string","index":0}`)
		got := f.run("data-table", "column", "update", "dt-1", "co/two?y=2", "--name", "state", "--index", "0", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("exit = %d, stderr: %s", got.code, got.stderr)
		}
		req := f.lastRequest()
		if req.Method != http.MethodPatch || req.URL.EscapedPath() != n8n.BasePath+"/data-tables/dt-1/columns/co%2Ftwo%3Fy=2" {
			t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
		}
		assertCLIJSON(t, `{"name":"state","index":0}`, f.lastBody())
		var column n8n.DataTableColumn
		if err := json.Unmarshal([]byte(got.stdout), &column); err != nil || column.Name != "state" {
			t.Errorf("stdout = %q, want the updated column (error: %v)", got.stdout, err)
		}
	})

	t.Run("delete confirmation", func(t *testing.T) {
		f := dataTableFixture(t, "")
		before := f.requestCount()
		f.stdin = "no\n"
		got := f.run("data-table", "column", "delete", "dt-1", "co-1")
		if got.code != ExitError || f.requestCount() != before {
			t.Errorf("declined result = %+v", got)
		}
		if !strings.Contains(got.stderr, "deleted in every row") {
			t.Errorf("stderr = %q, want the data-loss wording", got.stderr)
		}
		f.interactive = false
		got = f.run("data-table", "column", "delete", "dt-1", "co-1", "--yes", "--output", "json")
		if got.code != ExitSuccess {
			t.Fatalf("confirmed exit = %d, stderr: %s", got.code, got.stderr)
		}
		var result dataTableMutation
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil || result.Action != "column deleted" || result.ColumnID != "co-1" {
			t.Errorf("stdout = %q, want a column delete acknowledgement (error: %v)", got.stdout, err)
		}
	})
}

func TestDataTableValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{name: "unknown output", args: []string{"data-table", "list", "--output", "yaml"}, want: "unknown output format"},
		{name: "large limit", args: []string{"data-table", "list", "--limit", "251"}, want: "must not exceed 250"},
		{name: "malformed sort", args: []string{"data-table", "list", "--sort-by", "name"}, want: "field:asc"},
		{name: "create without columns", args: []string{"data-table", "create", "customers"}, want: "--column is required"},
		{name: "create malformed column", args: []string{"data-table", "create", "customers", "--column", "email"}, want: "NAME:TYPE"},
		{name: "create duplicate column", args: []string{"data-table", "create", "customers", "--column", "a:string", "--column", "a:number"}, want: "given twice"},
		{name: "create unknown type", args: []string{"data-table", "create", "customers", "--column", "a:text"}, want: "unknown column type"},
		{name: "blank get ID", args: []string{"data-table", "get", " "}, want: "data table ID is required"},
		{name: "update without name", args: []string{"data-table", "update", "dt-1"}, want: "--name is required"},
		{name: "row list malformed where", args: []string{"data-table", "row", "list", "dt-1", "--where", "status"}, want: "COLUMN=VALUE"},
		{name: "row list unknown match", args: []string{"data-table", "row", "list", "dt-1", "--where", "a=b", "--match", "xor"}, want: "neither and nor or"},
		{name: "row list where and filter", args: []string{"data-table", "row", "list", "dt-1", "--where", "a=b", "--filter", `{"filters":[]}`}, want: "cannot be used together"},
		{name: "row list bad filter JSON", args: []string{"data-table", "row", "list", "dt-1", "--filter", "status=active"}, want: "decode data table JSON"},
		{name: "row list unknown filter field", args: []string{"data-table", "row", "list", "dt-1", "--filter", `{"nope":1}`}, want: "unknown field"},
		{name: "row insert without values", args: []string{"data-table", "row", "insert", "dt-1"}, want: "--set or --input is required"},
		{name: "row insert malformed set", args: []string{"data-table", "row", "insert", "dt-1", "--set", "email"}, want: "COLUMN=VALUE"},
		{name: "row insert duplicate set", args: []string{"data-table", "row", "insert", "dt-1", "--set", "a=1", "--set", "a=2"}, want: "given twice"},
		{name: "row insert unknown return", args: []string{"data-table", "row", "insert", "dt-1", "--set", "a=1", "--return", "rows"}, want: "unknown return type"},
		{name: "row insert empty array", stdin: `[]`, args: []string{"data-table", "row", "insert", "dt-1", "--input", "-"}, want: "at least one row"},
		{name: "row insert two documents", stdin: `[{"a":1}] [{"a":2}]`, args: []string{"data-table", "row", "insert", "dt-1", "--input", "-"}, want: "multiple documents"},
		{name: "row update without filter", args: []string{"data-table", "row", "update", "dt-1", "--set", "a=1"}, want: "--where or --filter is required"},
		{name: "row update without values", args: []string{"data-table", "row", "update", "dt-1", "--where", "a=1"}, want: "--set or --input is required"},
		{name: "row update unknown condition", args: []string{"data-table", "row", "update", "dt-1", "--filter", `{"filters":[{"columnName":"a","condition":"between","value":1}]}`, "--set", "b=2"}, want: "unknown row filter condition"},
		{name: "row update empty filter document", args: []string{"data-table", "row", "update", "dt-1", "--filter", `{"filters":[]}`, "--set", "b=2"}, want: "at least one condition"},
		{name: "row upsert without filter", args: []string{"data-table", "row", "upsert", "dt-1", "--set", "a=1"}, want: "--where or --filter is required"},
		{name: "row delete without filter", args: []string{"data-table", "row", "delete", "dt-1", "--yes"}, want: "--where or --filter is required"},
		{name: "row clear blank table", args: []string{"data-table", "row", "clear", " ", "--yes"}, want: "data table ID is required"},
		{name: "column create unknown type", args: []string{"data-table", "column", "create", "dt-1", "meta", "json"}, want: "unknown column type"},
		{name: "column create negative index", args: []string{"data-table", "column", "create", "dt-1", "a", "string", "--index", "-1"}, want: "must not be negative"},
		{name: "column update without fields", args: []string{"data-table", "column", "update", "dt-1", "co-1"}, want: "--name or --index is required"},
		{name: "column delete blank column", args: []string{"data-table", "column", "delete", "dt-1", " ", "--yes"}, want: "column ID is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := dataTableFixture(t, `{"data":[],"nextCursor":null}`)
			f.interactive = false
			f.stdin = tt.stdin
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

func TestDataTableAPIErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		args   []string
		want   []string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, args: []string{"data-table", "list"}, want: []string{"401", "auth login"}},
		{name: "forbidden", status: http.StatusForbidden, args: []string{"data-table", "list"}, want: []string{"403", "discover --resource datatable"}},
		{name: "not found", status: http.StatusNotFound, args: []string{"data-table", "get", "dt-1"}, want: []string{"404", "n8n data-table list"}},
		{name: "conflict", status: http.StatusConflict, args: []string{"data-table", "update", "dt-1", "--name", "taken"}, want: []string{"409", "already exists"}},
		{name: "bad request", status: http.StatusBadRequest, args: []string{"data-table", "row", "list", "dt-1"}, want: []string{"400"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := dataTableFixture(t, "")
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
