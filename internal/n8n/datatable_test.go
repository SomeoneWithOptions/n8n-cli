package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const dataTableResponse = `{
  "id":"dt-1",
  "name":"customers",
  "projectId":"pr-1",
  "createdAt":"2026-09-17T19:31:31.471Z",
  "updatedAt":"2026-09-17T19:31:31.471Z",
  "sizeBytes":16384,
  "columns":[
    {"id":"co-1","dataTableId":"dt-1","name":"email","type":"string","index":0,
     "createdAt":"2026-09-17T19:31:31.471Z","updatedAt":"2026-09-17T19:31:31.471Z"}
  ],
  "fieldAddedLater":"ignored"
}`

const dataTableRowResponse = `{
  "id":1,
  "createdAt":"2026-09-17T19:31:42.152Z",
  "updatedAt":"2026-09-17T19:31:42.152Z",
  "email":"a@example.com",
  "age":30,
  "active":true,
  "seen":null
}`

func TestListDataTablesQueryAuthAndDecoding(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK,
		`{"data":[`+dataTableResponse+`],"nextCursor":"next/one?x=1"}`))
	page, err := server.client(t).ListDataTables(context.Background(), ListDataTablesOptions{
		ListOptions: ListOptions{Limit: 50, Cursor: "prev/one?x=1"},
		Filter:      DataTableFilter{Name: "customers"},
		SortBy:      "name:asc",
	})
	if err != nil {
		t.Fatalf("ListDataTables: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+DataTablesPath {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	query := req.URL.Query()
	if query.Get("limit") != "50" || query.Get("cursor") != "prev/one?x=1" || query.Get("sortBy") != "name:asc" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}
	assertJSONEqual(t, `{"name":"customers"}`, query.Get("filter"))
	if len(page.Data) != 1 || page.NextCursor != "next/one?x=1" {
		t.Fatalf("page = %+v", page)
	}
	table := page.Data[0]
	if table.ID != "dt-1" || table.SizeBytes != 16384 || table.ProjectID != "pr-1" {
		t.Errorf("table = %+v", table)
	}
	if len(table.Columns) != 1 || table.Columns[0].Name != "email" || table.Columns[0].Type != "string" {
		t.Errorf("columns = %+v", table.Columns)
	}
}

func TestListDataTablesDefaultQuery(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[],"nextCursor":null}`))
	if _, err := server.client(t).ListDataTables(context.Background(), ListDataTablesOptions{}); err != nil {
		t.Fatalf("ListDataTables: %v", err)
	}
	if got := server.requests[0].URL.RawQuery; got != "" {
		t.Errorf("query = %q, want empty", got)
	}
}

func TestDataTableRequests(t *testing.T) {
	tableID := "dt/one?x=1"
	columnID := "co/two?y=2"
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
			table, err := c.CreateDataTable(context.Background(), CreateDataTableRequest{
				Name:      "customers",
				ProjectID: "pr-1",
				Columns: []DataTableColumnDefinition{
					{Name: "email", Type: "string"},
					{Name: "age", Type: "number"},
				},
			})
			if err == nil && table.ID != "dt-1" {
				t.Errorf("table = %+v", table)
			}
			return err
		}, method: http.MethodPost, path: "/data-tables", status: http.StatusCreated, result: dataTableResponse,
			body: `{"name":"customers","columns":[{"name":"email","type":"string"},{"name":"age","type":"number"}],"projectId":"pr-1"}`},
		{name: "get", call: func(c *Client) error {
			_, err := c.GetDataTable(context.Background(), tableID)
			return err
		}, method: http.MethodGet, path: "/data-tables/dt%2Fone%3Fx=1", status: http.StatusOK, result: dataTableResponse},
		{name: "update", call: func(c *Client) error {
			_, err := c.UpdateDataTable(context.Background(), tableID, "renamed")
			return err
		}, method: http.MethodPatch, path: "/data-tables/dt%2Fone%3Fx=1", status: http.StatusOK,
			body: `{"name":"renamed"}`, result: dataTableResponse},
		{name: "delete", call: func(c *Client) error {
			return c.DeleteDataTable(context.Background(), tableID)
		}, method: http.MethodDelete, path: "/data-tables/dt%2Fone%3Fx=1", status: http.StatusNoContent},
		{name: "row list", call: func(c *Client) error {
			page, err := c.ListDataTableRows(context.Background(), tableID, ListRowsOptions{
				ListOptions: ListOptions{Limit: 2},
				Filter: RowFilter{Type: "and", Filters: []RowCondition{
					{ColumnName: "age", Condition: "gte", Value: json.RawMessage(`30`)},
				}},
				SortBy: "age:desc",
				Search: "example",
			})
			if err == nil {
				if len(page.Data) != 1 || page.Data[0].Text("email") != "a@example.com" {
					t.Errorf("page = %+v", page)
				}
			}
			return err
		}, method: http.MethodGet, path: "/data-tables/dt%2Fone%3Fx=1/rows", status: http.StatusOK,
			query:  `filter=%7B%22type%22%3A%22and%22%2C%22filters%22%3A%5B%7B%22columnName%22%3A%22age%22%2C%22condition%22%3A%22gte%22%2C%22value%22%3A30%7D%5D%7D&limit=2&search=example&sortBy=age%3Adesc`,
			result: `{"data":[` + dataTableRowResponse + `],"nextCursor":null}`},
		{name: "row clear", call: func(c *Client) error {
			result, err := c.ClearDataTableRows(context.Background(), tableID)
			if err == nil && result.DeletedCount != 3 {
				t.Errorf("result = %+v, want three deleted rows", result)
			}
			return err
		}, method: http.MethodDelete, path: "/data-tables/dt%2Fone%3Fx=1/rows/clear", status: http.StatusOK,
			result: `{"deletedCount":3}`},
		{name: "column list", call: func(c *Client) error {
			columns, err := c.ListDataTableColumns(context.Background(), tableID)
			if err == nil && (len(columns) != 1 || columns[0].ID != "co-1" || columns[0].Index != 0) {
				t.Errorf("columns = %+v", columns)
			}
			return err
		}, method: http.MethodGet, path: "/data-tables/dt%2Fone%3Fx=1/columns", status: http.StatusOK,
			result: `[{"id":"co-1","dataTableId":"dt-1","name":"email","type":"string","index":0}]`},
		{name: "column create", call: func(c *Client) error {
			_, err := c.CreateDataTableColumn(context.Background(), tableID, CreateColumnRequest{
				Name: "status", Type: "string", Index: intPtr(1),
			})
			return err
		}, method: http.MethodPost, path: "/data-tables/dt%2Fone%3Fx=1/columns", status: http.StatusCreated,
			body:   `{"name":"status","type":"string","index":1}`,
			result: `{"id":"co-2","dataTableId":"dt-1","name":"status","type":"string","index":1}`},
		{name: "column create appended", call: func(c *Client) error {
			_, err := c.CreateDataTableColumn(context.Background(), tableID, CreateColumnRequest{Name: "status", Type: "string"})
			return err
		}, method: http.MethodPost, path: "/data-tables/dt%2Fone%3Fx=1/columns", status: http.StatusCreated,
			body:   `{"name":"status","type":"string"}`,
			result: `{"id":"co-2","dataTableId":"dt-1","name":"status","type":"string","index":1}`},
		{name: "column update", call: func(c *Client) error {
			_, err := c.UpdateDataTableColumn(context.Background(), tableID, columnID, UpdateColumnRequest{
				Name: "state", Index: intPtr(0),
			})
			return err
		}, method: http.MethodPatch, path: "/data-tables/dt%2Fone%3Fx=1/columns/co%2Ftwo%3Fy=2", status: http.StatusOK,
			body:   `{"name":"state","index":0}`,
			result: `{"id":"co-2","dataTableId":"dt-1","name":"state","type":"string","index":0}`},
		{name: "column delete", call: func(c *Client) error {
			return c.DeleteDataTableColumn(context.Background(), tableID, columnID)
		}, method: http.MethodDelete, path: "/data-tables/dt%2Fone%3Fx=1/columns/co%2Ftwo%3Fy=2", status: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(tt.status, tt.result))
			if err := tt.call(server.client(t)); err != nil {
				t.Fatalf("call: %v", err)
			}
			req, gotBody := server.last(t)
			if req.Method != tt.method || req.URL.EscapedPath() != BasePath+tt.path {
				t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.EscapedPath(), tt.method, BasePath+tt.path)
			}
			if req.URL.RawQuery != tt.query {
				t.Errorf("query = %q, want %q", req.URL.RawQuery, tt.query)
			}
			if tt.body == "" {
				if gotBody != "" {
					t.Errorf("body = %q, want empty", gotBody)
				}
			} else {
				assertJSONEqual(t, tt.body, gotBody)
			}
		})
	}
}

func TestInsertDataTableRowsPreservesEveryReturnShape(t *testing.T) {
	tests := []struct {
		name   string
		result string
		check  func(*testing.T, *InsertRowsResult)
	}{
		{name: "documented count", result: `{"count":2}`, check: func(t *testing.T, r *InsertRowsResult) {
			if r.Count == nil || *r.Count != 2 || len(r.Rows) != 0 {
				t.Errorf("result = %+v", r)
			}
		}},
		{name: "instance count", result: `{"success":true,"insertedRows":1}`, check: func(t *testing.T, r *InsertRowsResult) {
			if r.Count == nil || *r.Count != 1 || !r.Acknowledged {
				t.Errorf("result = %+v", r)
			}
		}},
		{name: "bare IDs", result: `[1,2,3]`, check: func(t *testing.T, r *InsertRowsResult) {
			if len(r.IDs) != 3 || r.IDs[2] != 3 || len(r.Rows) != 0 {
				t.Errorf("result = %+v", r)
			}
		}},
		{name: "ID objects", result: `[{"id":2}]`, check: func(t *testing.T, r *InsertRowsResult) {
			if len(r.IDs) != 1 || r.IDs[0] != 2 || len(r.Rows) != 1 {
				t.Errorf("result = %+v", r)
			}
		}},
		{name: "full rows", result: `[` + dataTableRowResponse + `]`, check: func(t *testing.T, r *InsertRowsResult) {
			if len(r.Rows) != 1 || r.Rows[0].Text("email") != "a@example.com" || len(r.IDs) != 1 {
				t.Errorf("result = %+v", r)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, tt.result))
			result, err := server.client(t).InsertDataTableRows(context.Background(), "dt-1", InsertRowsRequest{
				Data:       []DataTableRow{{"email": json.RawMessage(`"a@example.com"`), "age": json.RawMessage(`30`)}},
				ReturnType: InsertReturnAll,
			})
			if err != nil {
				t.Fatalf("InsertDataTableRows: %v", err)
			}
			req, body := server.last(t)
			if req.Method != http.MethodPost || req.URL.EscapedPath() != BasePath+"/data-tables/dt-1/rows" {
				t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
			}
			assertJSONEqual(t, `{"data":[{"email":"a@example.com","age":30}],"returnType":"all"}`, body)
			tt.check(t, result)
		})
	}
}

func TestRowMutationResultsPreserveEveryShape(t *testing.T) {
	dryRun := `[{"id":1,"age":30,"dryRunState":"before"},{"id":1,"age":31,"dryRunState":"after"}]`
	filter := RowFilter{Filters: []RowCondition{{ColumnName: "email", Condition: "eq", Value: json.RawMessage(`"a@example.com"`)}}}
	tests := []struct {
		name   string
		result string
		call   func(*Client) (*RowMutationResult, error)
		method string
		path   string
		query  string
		body   string
		check  func(*testing.T, *RowMutationResult)
	}{
		{name: "update acknowledged", result: `true`, method: http.MethodPatch, path: "/data-tables/dt-1/rows/update",
			body: `{"filter":{"filters":[{"columnName":"email","condition":"eq","value":"a@example.com"}]},"data":{"age":31}}`,
			call: func(c *Client) (*RowMutationResult, error) {
				return c.UpdateDataTableRows(context.Background(), "dt-1", UpdateRowsRequest{
					Filter: filter, Data: DataTableRow{"age": json.RawMessage(`31`)},
				})
			},
			check: func(t *testing.T, r *RowMutationResult) {
				if !r.Acknowledged || len(r.Rows) != 0 {
					t.Errorf("result = %+v", r)
				}
			}},
		{name: "update dry run rows", result: dryRun, method: http.MethodPatch, path: "/data-tables/dt-1/rows/update",
			body: `{"filter":{"filters":[{"columnName":"email","condition":"eq","value":"a@example.com"}]},"data":{"age":31},"returnData":true,"dryRun":true}`,
			call: func(c *Client) (*RowMutationResult, error) {
				return c.UpdateDataTableRows(context.Background(), "dt-1", UpdateRowsRequest{
					Filter: filter, Data: DataTableRow{"age": json.RawMessage(`31`)}, ReturnData: true, DryRun: true,
				})
			},
			check: func(t *testing.T, r *RowMutationResult) {
				if len(r.Rows) != 2 || r.Rows[0].Text("dryRunState") != "before" || r.Rows[1].Text("dryRunState") != "after" {
					t.Errorf("result = %+v, want the dry-run before and after rows", r)
				}
			}},
		{name: "upsert single object", result: dataTableRowResponse, method: http.MethodPost, path: "/data-tables/dt-1/rows/upsert",
			body: `{"filter":{"filters":[{"columnName":"email","condition":"eq","value":"a@example.com"}]},"data":{"age":31},"returnData":true}`,
			call: func(c *Client) (*RowMutationResult, error) {
				return c.UpsertDataTableRow(context.Background(), "dt-1", UpsertRowRequest{
					Filter: filter, Data: DataTableRow{"age": json.RawMessage(`31`)}, ReturnData: true,
				})
			},
			check: func(t *testing.T, r *RowMutationResult) {
				if len(r.Rows) != 1 || r.Rows[0].Text("email") != "a@example.com" {
					t.Errorf("result = %+v, want the single upserted row", r)
				}
			}},
		{name: "upsert array", result: `[` + dataTableRowResponse + `]`, method: http.MethodPost, path: "/data-tables/dt-1/rows/upsert",
			body: `{"filter":{"filters":[{"columnName":"email","condition":"eq","value":"a@example.com"}]},"data":{"age":31},"returnData":true}`,
			call: func(c *Client) (*RowMutationResult, error) {
				return c.UpsertDataTableRow(context.Background(), "dt-1", UpsertRowRequest{
					Filter: filter, Data: DataTableRow{"age": json.RawMessage(`31`)}, ReturnData: true,
				})
			},
			check: func(t *testing.T, r *RowMutationResult) {
				if len(r.Rows) != 1 {
					t.Errorf("result = %+v", r)
				}
			}},
		{name: "delete acknowledged", result: `true`, method: http.MethodDelete, path: "/data-tables/dt-1/rows/delete",
			query: `filter=%7B%22filters%22%3A%5B%7B%22columnName%22%3A%22email%22%2C%22condition%22%3A%22eq%22%2C%22value%22%3A%22a%40example.com%22%7D%5D%7D`,
			call: func(c *Client) (*RowMutationResult, error) {
				return c.DeleteDataTableRows(context.Background(), "dt-1", DeleteRowsOptions{Filter: filter})
			},
			check: func(t *testing.T, r *RowMutationResult) {
				if !r.Acknowledged {
					t.Errorf("result = %+v", r)
				}
			}},
		{name: "delete dry run", result: dryRun, method: http.MethodDelete, path: "/data-tables/dt-1/rows/delete",
			query: `dryRun=true&filter=%7B%22filters%22%3A%5B%7B%22columnName%22%3A%22email%22%2C%22condition%22%3A%22eq%22%2C%22value%22%3A%22a%40example.com%22%7D%5D%7D&returnData=true`,
			call: func(c *Client) (*RowMutationResult, error) {
				return c.DeleteDataTableRows(context.Background(), "dt-1", DeleteRowsOptions{Filter: filter, ReturnData: true, DryRun: true})
			},
			check: func(t *testing.T, r *RowMutationResult) {
				if len(r.Rows) != 2 {
					t.Errorf("result = %+v", r)
				}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, tt.result))
			result, err := tt.call(server.client(t))
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			req, body := server.last(t)
			if req.Method != tt.method || req.URL.EscapedPath() != BasePath+tt.path {
				t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.EscapedPath(), tt.method, BasePath+tt.path)
			}
			if req.URL.RawQuery != tt.query {
				t.Errorf("query = %q, want %q", req.URL.RawQuery, tt.query)
			}
			if tt.body == "" {
				if body != "" {
					t.Errorf("body = %q, want empty", body)
				}
			} else {
				assertJSONEqual(t, tt.body, body)
			}
			tt.check(t, result)
		})
	}
}

func TestDataTablePaginationCollectsEveryPage(t *testing.T) {
	pages := 0
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		if pages == 0 {
			pages++
			_, _ = w.Write([]byte(`{"data":[` + dataTableResponse + `],"nextCursor":"second"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"dt-2","name":"orders"}],"nextCursor":null}`))
	})
	client := server.client(t)
	fetch := func(ctx context.Context, opts ListOptions) (Page[DataTable], error) {
		return client.ListDataTables(ctx, ListDataTablesOptions{ListOptions: opts})
	}
	tables, err := Collect(context.Background(), fetch, ListOptions{Limit: 1}, 0)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(tables) != 2 || tables[1].ID != "dt-2" {
		t.Errorf("tables = %+v, want both pages", tables)
	}
	if got := server.requests[1].URL.Query().Get("cursor"); got != "second" {
		t.Errorf("second cursor = %q", got)
	}
}

func TestDataTableRowPaginationCollectsEveryPage(t *testing.T) {
	pages := 0
	server := newCommunityPackageServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		if pages == 0 {
			pages++
			_, _ = w.Write([]byte(`{"data":[` + dataTableRowResponse + `],"nextCursor":"second"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":2}],"nextCursor":null}`))
	})
	client := server.client(t)
	fetch := func(ctx context.Context, opts ListOptions) (Page[DataTableRow], error) {
		return client.ListDataTableRows(ctx, "dt-1", ListRowsOptions{ListOptions: opts, Search: "example"})
	}
	rows, err := Collect(context.Background(), fetch, ListOptions{Limit: 1}, 0)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want both pages", rows)
	}
	if id, ok := rows[1].RowID(); !ok || id != 2 {
		t.Errorf("second row ID = %d (%v)", id, ok)
	}
	// The filter must survive page two, or later pages silently widen.
	if got := server.requests[1].URL.Query().Get("search"); got != "example" {
		t.Errorf("second page search = %q, want it preserved", got)
	}
}

func TestDataTableErrorsRemainStructured(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusBadRequest,
		`{"message":"request/query must have required property 'filter'"}`))
	_, err := server.client(t).ListDataTables(context.Background(), ListDataTablesOptions{})
	if StatusCodeOf(err) != http.StatusBadRequest || !strings.Contains(err.Error(), "filter") {
		t.Errorf("error = %v, want a structured 400", err)
	}
}

func TestDataTableRowHelpers(t *testing.T) {
	var row DataTableRow
	if err := json.Unmarshal([]byte(dataTableRowResponse), &row); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if id, ok := row.RowID(); !ok || id != 1 {
		t.Errorf("RowID = %d (%v), want 1", id, ok)
	}
	if got := row.Text("email"); got != "a@example.com" {
		t.Errorf("Text(email) = %q", got)
	}
	if got := row.Text("age"); got != "30" {
		t.Errorf("Text(age) = %q", got)
	}
	if got := row.Text("seen"); got != "" {
		t.Errorf("Text(seen) = %q, want empty for null", got)
	}
	if got := row.Text("missing"); got != "" {
		t.Errorf("Text(missing) = %q, want empty", got)
	}
	want := []string{"id", "createdAt", "updatedAt", "active", "age", "email", "seen"}
	got := row.Columns()
	if len(got) != len(want) {
		t.Fatalf("Columns() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Columns() = %v, want %v", got, want)
		}
	}
}

func TestJSONValueKeepsTypes(t *testing.T) {
	tests := map[string]string{
		"30":             `30`,
		"-1.5":           `-1.5`,
		"true":           `true`,
		"null":           `null`,
		`{"a":1}`:        `{"a":1}`,
		`[1,2]`:          `[1,2]`,
		`"quoted"`:       `"quoted"`,
		"a@example.com":  `"a@example.com"`,
		"active":         `"active"`,
		"2026-01-01":     `"2026-01-01"`,
		"":               `""`,
		"with spaces {}": `"with spaces {}"`,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := string(JSONValue(input)); got != want {
				t.Errorf("JSONValue(%q) = %s, want %s", input, got, want)
			}
		})
	}
}

func TestDataTableValidationBeforeRequest(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{}`))
	client := server.client(t)
	ctx := context.Background()
	condition := RowCondition{ColumnName: "email", Condition: "eq", Value: json.RawMessage(`"a"`)}
	filter := RowFilter{Filters: []RowCondition{condition}}
	calls := map[string]func() error{
		"large limit": func() error {
			_, err := client.ListDataTables(ctx, ListDataTablesOptions{ListOptions: ListOptions{Limit: 251}})
			return err
		},
		"malformed sort": func() error {
			_, err := client.ListDataTables(ctx, ListDataTablesOptions{SortBy: "name"})
			return err
		},
		"blank create name": func() error {
			_, err := client.CreateDataTable(ctx, CreateDataTableRequest{Name: " ", Columns: []DataTableColumnDefinition{{Name: "a", Type: "string"}}})
			return err
		},
		"create without columns": func() error {
			_, err := client.CreateDataTable(ctx, CreateDataTableRequest{Name: "customers"})
			return err
		},
		"unknown column type": func() error {
			_, err := client.CreateDataTable(ctx, CreateDataTableRequest{Name: "customers", Columns: []DataTableColumnDefinition{{Name: "a", Type: "text"}}})
			return err
		},
		"overlong name": func() error {
			_, err := client.CreateDataTable(ctx, CreateDataTableRequest{
				Name:    strings.Repeat("a", 129),
				Columns: []DataTableColumnDefinition{{Name: "a", Type: "string"}},
			})
			return err
		},
		"blank get ID": func() error {
			_, err := client.GetDataTable(ctx, " ")
			return err
		},
		"blank update name": func() error {
			_, err := client.UpdateDataTable(ctx, "dt-1", "")
			return err
		},
		"padded delete ID": func() error {
			return client.DeleteDataTable(ctx, " dt-1")
		},
		"row list bad filter": func() error {
			_, err := client.ListDataTableRows(ctx, "dt-1", ListRowsOptions{
				Filter: RowFilter{Filters: []RowCondition{{ColumnName: "a", Condition: "between", Value: json.RawMessage(`1`)}}},
			})
			return err
		},
		"row list bad filter type": func() error {
			_, err := client.ListDataTableRows(ctx, "dt-1", ListRowsOptions{Filter: RowFilter{Type: "xor", Filters: []RowCondition{condition}}})
			return err
		},
		"insert without rows": func() error {
			_, err := client.InsertDataTableRows(ctx, "dt-1", InsertRowsRequest{})
			return err
		},
		"insert empty row": func() error {
			_, err := client.InsertDataTableRows(ctx, "dt-1", InsertRowsRequest{Data: []DataTableRow{{}}})
			return err
		},
		"insert unknown return type": func() error {
			_, err := client.InsertDataTableRows(ctx, "dt-1", InsertRowsRequest{
				Data: []DataTableRow{{"a": json.RawMessage(`1`)}}, ReturnType: "rows",
			})
			return err
		},
		"update without filter": func() error {
			_, err := client.UpdateDataTableRows(ctx, "dt-1", UpdateRowsRequest{Data: DataTableRow{"a": json.RawMessage(`1`)}})
			return err
		},
		"update without data": func() error {
			_, err := client.UpdateDataTableRows(ctx, "dt-1", UpdateRowsRequest{Filter: filter})
			return err
		},
		"upsert without filter": func() error {
			_, err := client.UpsertDataTableRow(ctx, "dt-1", UpsertRowRequest{Data: DataTableRow{"a": json.RawMessage(`1`)}})
			return err
		},
		"delete without filter": func() error {
			_, err := client.DeleteDataTableRows(ctx, "dt-1", DeleteRowsOptions{})
			return err
		},
		"clear blank table": func() error {
			_, err := client.ClearDataTableRows(ctx, "")
			return err
		},
		"column create unknown type": func() error {
			_, err := client.CreateDataTableColumn(ctx, "dt-1", CreateColumnRequest{Name: "a", Type: "json"})
			return err
		},
		"column create negative index": func() error {
			_, err := client.CreateDataTableColumn(ctx, "dt-1", CreateColumnRequest{Name: "a", Type: "string", Index: intPtr(-1)})
			return err
		},
		"column update empty": func() error {
			_, err := client.UpdateDataTableColumn(ctx, "dt-1", "co-1", UpdateColumnRequest{})
			return err
		},
		"column update blank column": func() error {
			_, err := client.UpdateDataTableColumn(ctx, "dt-1", " ", UpdateColumnRequest{Name: "a"})
			return err
		},
		"column delete blank column": func() error {
			return client.DeleteDataTableColumn(ctx, "dt-1", "")
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Error("call succeeded, want validation error")
			}
		})
	}
	if len(server.requests) != 0 {
		t.Errorf("request count = %d, want 0", len(server.requests))
	}
}

func intPtr(v int) *int { return &v }
