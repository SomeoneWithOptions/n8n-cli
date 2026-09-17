package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

// TestListDataTables is read-only. It checks cursor paging and the table shape
// against the live contract without creating anything.
func TestListDataTables(t *testing.T) {
	instance := integration.Require(t)
	page, err := instance.Client(t).ListDataTables(context.Background(), n8n.ListDataTablesOptions{
		ListOptions: n8n.ListOptions{Limit: 10},
		SortBy:      "name:asc",
	})
	if err != nil {
		t.Fatalf("ListDataTables: %v", err)
	}
	for _, table := range page.Data {
		if table.ID == "" || table.Name == "" {
			t.Errorf("data table is missing its ID or name: %+v", table)
		}
	}
	t.Logf("instance returned %d data table(s)", len(page.Data))
}

// TestDataTableLifecycle creates one prefixed table, exercises every row and
// column operation on it, and deletes it again. It only ever touches the table
// it created itself.
func TestDataTableLifecycle(t *testing.T) {
	instance := integration.RequireDestructive(t)
	client := instance.Client(t)
	ctx := context.Background()
	name := fmt.Sprintf("%stable-%d", integration.ResourcePrefix, time.Now().UnixNano())

	table, err := client.CreateDataTable(ctx, n8n.CreateDataTableRequest{
		Name: name,
		Columns: []n8n.DataTableColumnDefinition{
			{Name: "email", Type: "string"},
			{Name: "age", Type: "number"},
		},
	})
	if err != nil {
		t.Fatalf("CreateDataTable(%q): %v", name, err)
	}
	if table.ID == "" || len(table.Columns) != 2 {
		t.Fatalf("created table = %+v, want an ID and two columns", table)
	}
	t.Cleanup(func() {
		if err := client.DeleteDataTable(context.Background(), table.ID); err != nil && !n8n.IsNotFound(err) {
			t.Errorf("cleanup DeleteDataTable(%q): %v", table.ID, err)
		}
	})

	// Rows: insert, read back, update with a dry run first, upsert, delete.
	inserted, err := client.InsertDataTableRows(ctx, table.ID, n8n.InsertRowsRequest{
		Data: []n8n.DataTableRow{
			{"email": n8n.JSONValue("a@example.com"), "age": n8n.JSONValue("30")},
			{"email": n8n.JSONValue("b@example.com"), "age": n8n.JSONValue("25")},
		},
		ReturnType: n8n.InsertReturnAll,
	})
	if err != nil {
		t.Fatalf("InsertDataTableRows: %v", err)
	}
	if len(inserted.Rows) != 2 || len(inserted.IDs) != 2 {
		t.Fatalf("insert result = %+v, want two rows with IDs", inserted)
	}

	rows, err := client.ListDataTableRows(ctx, table.ID, n8n.ListRowsOptions{
		Filter: n8n.RowFilter{Filters: []n8n.RowCondition{
			{ColumnName: "age", Condition: "gte", Value: n8n.JSONValue("30")},
		}},
		SortBy: "age:desc",
	})
	if err != nil {
		t.Fatalf("ListDataTableRows: %v", err)
	}
	if len(rows.Data) != 1 || rows.Data[0].Text("email") != "a@example.com" {
		t.Errorf("filtered rows = %+v, want only the older row", rows.Data)
	}

	byEmail := n8n.RowFilter{Filters: []n8n.RowCondition{
		{ColumnName: "email", Condition: "eq", Value: n8n.JSONValue("a@example.com")},
	}}
	dryRun, err := client.UpdateDataTableRows(ctx, table.ID, n8n.UpdateRowsRequest{
		Filter: byEmail, Data: n8n.DataTableRow{"age": n8n.JSONValue("31")}, ReturnData: true, DryRun: true,
	})
	if err != nil {
		t.Fatalf("UpdateDataTableRows(dry run): %v", err)
	}
	if len(dryRun.Rows) == 0 {
		t.Error("dry run returned no rows, want the before and after states")
	}
	unchanged, err := client.ListDataTableRows(ctx, table.ID, n8n.ListRowsOptions{Filter: byEmail})
	if err != nil {
		t.Fatalf("ListDataTableRows after dry run: %v", err)
	}
	if len(unchanged.Data) != 1 || unchanged.Data[0].Text("age") != "30" {
		t.Errorf("dry run changed the data: %+v", unchanged.Data)
	}

	if _, err := client.UpdateDataTableRows(ctx, table.ID, n8n.UpdateRowsRequest{
		Filter: byEmail, Data: n8n.DataTableRow{"age": n8n.JSONValue("31")},
	}); err != nil {
		t.Fatalf("UpdateDataTableRows: %v", err)
	}

	upserted, err := client.UpsertDataTableRow(ctx, table.ID, n8n.UpsertRowRequest{
		Filter: n8n.RowFilter{Filters: []n8n.RowCondition{
			{ColumnName: "email", Condition: "eq", Value: n8n.JSONValue("c@example.com")},
		}},
		Data:       n8n.DataTableRow{"email": n8n.JSONValue("c@example.com"), "age": n8n.JSONValue("40")},
		ReturnData: true,
	})
	if err != nil {
		t.Fatalf("UpsertDataTableRow: %v", err)
	}
	if len(upserted.Rows) != 1 {
		t.Errorf("upsert result = %+v, want the inserted row", upserted)
	}

	deleted, err := client.DeleteDataTableRows(ctx, table.ID, n8n.DeleteRowsOptions{
		Filter: n8n.RowFilter{Filters: []n8n.RowCondition{
			{ColumnName: "email", Condition: "eq", Value: n8n.JSONValue("c@example.com")},
		}},
	})
	if err != nil {
		t.Fatalf("DeleteDataTableRows: %v", err)
	}
	if !deleted.Acknowledged {
		t.Errorf("delete result = %+v, want an acknowledgement", deleted)
	}

	// Columns: add, rename, reorder, remove.
	column, err := client.CreateDataTableColumn(ctx, table.ID, n8n.CreateColumnRequest{Name: "status", Type: "string"})
	if err != nil {
		t.Fatalf("CreateDataTableColumn: %v", err)
	}
	index := 0
	renamed, err := client.UpdateDataTableColumn(ctx, table.ID, column.ID, n8n.UpdateColumnRequest{Name: "state", Index: &index})
	if err != nil {
		t.Fatalf("UpdateDataTableColumn: %v", err)
	}
	if renamed.Name != "state" || renamed.Index != 0 {
		t.Errorf("updated column = %+v, want it renamed and first", renamed)
	}
	columns, err := client.ListDataTableColumns(ctx, table.ID)
	if err != nil {
		t.Fatalf("ListDataTableColumns: %v", err)
	}
	if len(columns) != 3 {
		t.Errorf("columns = %+v, want three", columns)
	}
	if err := client.DeleteDataTableColumn(ctx, table.ID, column.ID); err != nil {
		t.Fatalf("DeleteDataTableColumn: %v", err)
	}

	// Clearing keeps the table and reports what it removed.
	cleared, err := client.ClearDataTableRows(ctx, table.ID)
	if err != nil {
		t.Fatalf("ClearDataTableRows: %v", err)
	}
	if cleared.DeletedCount == 0 {
		t.Errorf("cleared %d rows, want the remaining ones", cleared.DeletedCount)
	}
	remaining, err := client.ListDataTableRows(ctx, table.ID, n8n.ListRowsOptions{})
	if err != nil {
		t.Fatalf("ListDataTableRows after clear: %v", err)
	}
	if len(remaining.Data) != 0 {
		t.Errorf("rows after clear = %+v, want none", remaining.Data)
	}

	renamedTable, err := client.UpdateDataTable(ctx, table.ID, name+"-renamed")
	if err != nil {
		t.Fatalf("UpdateDataTable: %v", err)
	}
	if renamedTable.Name != name+"-renamed" {
		t.Errorf("table name = %q, want it renamed", renamedTable.Name)
	}

	if err := client.DeleteDataTable(ctx, table.ID); err != nil {
		t.Fatalf("DeleteDataTable: %v", err)
	}
	if _, err := client.GetDataTable(ctx, table.ID); !n8n.IsNotFound(err) {
		t.Errorf("GetDataTable after delete = %v, want a 404", err)
	}
}

// TestDataTableListCommand runs read-only listing through the full CLI.
func TestDataTableListCommand(t *testing.T) {
	instance := integration.Require(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"data-table", "list", "--limit", "10", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("data-table list exit code = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}
	var page n8n.Page[n8n.DataTable]
	if err := json.Unmarshal([]byte(out.String()), &page); err != nil {
		t.Fatalf("stdout is not a data table page: %v\n%s", err, out.String())
	}
	for _, table := range page.Data {
		if table.ID == "" {
			t.Errorf("data table omitted its ID: %+v", table)
		}
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
