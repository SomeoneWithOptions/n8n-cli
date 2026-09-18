package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// DataTablesPath is the collection endpoint for data tables.
const DataTablesPath = "/data-tables"

// maxDataTablePageSize is the page size the API documents as its maximum.
const maxDataTablePageSize = 250

// maxDataTableNameLength is the documented name limit for a table.
const maxDataTableNameLength = 128

// DataTableColumnTypes are the column types an existing table accepts.
var DataTableColumnTypes = []string{"string", "number", "boolean", "date"}

// DataTableCreateColumnTypes are the column types the create-table request
// documents. It adds "json", which some instances reject with a 400.
var DataTableCreateColumnTypes = []string{"string", "number", "boolean", "date", "json"}

// DataTableRowConditions are the comparison operators of a row filter.
var DataTableRowConditions = []string{"eq", "neq", "like", "ilike", "gt", "gte", "lt", "lte"}

// DataTableFilterTypes are the ways a row filter combines its conditions.
var DataTableFilterTypes = []string{"and", "or"}

// DataTableInsertReturnTypes are the documented shapes an insert can return.
const (
	InsertReturnCount = "count"
	InsertReturnID    = "id"
	InsertReturnAll   = "all"
)

// DataTableInsertReturnTypes lists [InsertReturnCount], [InsertReturnID] and
// [InsertReturnAll].
var DataTableInsertReturnTypes = []string{InsertReturnCount, InsertReturnID, InsertReturnAll}

// sortByPattern is the documented sort format, field:asc or field:desc. The
// API does not document which fields are sortable, so only the shape is
// checked here and an unknown field comes back as a 400.
var sortByPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*:(asc|desc)$`)

// DataTable is one data table with its column definitions.
type DataTable struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Columns   []DataTableColumn `json:"columns,omitempty"`
	ProjectID string            `json:"projectId,omitempty"`
	CreatedAt string            `json:"createdAt,omitempty"`
	UpdatedAt string            `json:"updatedAt,omitempty"`
	// SizeBytes is physical storage including indexes. It is not reduced
	// immediately by row deletion and may be a few seconds stale.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

// DataTableColumn is one column of a data table. The timestamps are not in the
// documented schema but every instance sends them.
type DataTableColumn struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	DataTableID string `json:"dataTableId,omitempty"`
	Type        string `json:"type,omitempty"`
	Index       int    `json:"index"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

// DataTableRow is one row: the system columns id, createdAt and updatedAt plus
// the user-defined ones, which differ per table. Values stay as raw JSON so
// numbers, dates, nulls and nested documents survive a round trip unchanged,
// and so a dry run's extra dryRunState marker is preserved rather than dropped.
type DataTableRow map[string]json.RawMessage

// RowID returns the row's numeric ID when it has one.
func (r DataTableRow) RowID() (int64, bool) {
	raw, ok := r["id"]
	if !ok {
		return 0, false
	}
	var id int64
	if err := json.Unmarshal(raw, &id); err != nil {
		return 0, false
	}
	return id, true
}

// Text renders one value for display: a JSON string without its quotes, a null
// as the empty string, and anything else as its JSON form.
func (r DataTableRow) Text(column string) string {
	raw, ok := r[column]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return string(raw)
}

// dataTableSystemColumns are the system columns Columns reports first.
var dataTableSystemColumns = []string{"id", "createdAt", "updatedAt"}

// Columns returns the row's column names in a stable order: the system columns
// first, then the user-defined ones sorted by name.
func (r DataTableRow) Columns() []string {
	names := make([]string, 0, len(r))
	for name := range r {
		if !slices.Contains(dataTableSystemColumns, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	ordered := make([]string, 0, len(names)+len(dataTableSystemColumns))
	for _, name := range dataTableSystemColumns {
		if _, ok := r[name]; ok {
			ordered = append(ordered, name)
		}
	}
	return append(ordered, names...)
}

// DataTableFilter is the documented filter of the table list endpoint. Name is
// the only field the contract documents.
type DataTableFilter struct {
	Name string `json:"name,omitempty"`
}

// IsZero reports whether the filter narrows nothing.
func (f DataTableFilter) IsZero() bool { return f.Name == "" }

// ListDataTablesOptions are the query parameters of the table list endpoint.
type ListDataTablesOptions struct {
	ListOptions
	// Filter narrows the tables returned. The zero value sends no filter.
	Filter DataTableFilter
	// SortBy is "field:asc" or "field:desc", e.g. "name:asc" or "size:desc".
	SortBy string
}

// Validate rejects options the API would reject.
func (o ListDataTablesOptions) Validate() error {
	if err := validateDataTablePagination(o.ListOptions); err != nil {
		return err
	}
	if err := validateSortBy(o.SortBy); err != nil {
		return err
	}
	return validateDataTableText("filter name", o.Filter.Name, false)
}

func (o ListDataTablesOptions) apply() (url.Values, error) {
	query := o.ListOptions.Apply(nil)
	if o.SortBy != "" {
		query.Set("sortBy", o.SortBy)
	}
	if !o.Filter.IsZero() {
		encoded, err := json.Marshal(o.Filter)
		if err != nil {
			return nil, fmt.Errorf("encode data table filter: %w", err)
		}
		query.Set("filter", string(encoded))
	}
	return query, nil
}

// RowCondition is one comparison inside a row filter.
type RowCondition struct {
	ColumnName string          `json:"columnName"`
	Condition  string          `json:"condition"`
	Value      json.RawMessage `json:"value"`
}

// Validate rejects a malformed condition before transport.
func (c RowCondition) Validate() error {
	if err := validateDataTableText("filter column name", c.ColumnName, true); err != nil {
		return err
	}
	if !slices.Contains(DataTableRowConditions, c.Condition) {
		return fmt.Errorf("unknown row filter condition %q: the API defines %s", c.Condition, strings.Join(DataTableRowConditions, ", "))
	}
	if len(c.Value) == 0 {
		return fmt.Errorf("row filter condition on %q needs a value", c.ColumnName)
	}
	if !json.Valid(c.Value) {
		return fmt.Errorf("row filter value for %q is not valid JSON", c.ColumnName)
	}
	return nil
}

// RowFilter selects the rows an operation applies to. Type defaults to "and"
// on the server when it is empty.
type RowFilter struct {
	Type    string         `json:"type,omitempty"`
	Filters []RowCondition `json:"filters"`
}

// IsZero reports whether the filter selects nothing in particular.
func (f RowFilter) IsZero() bool { return len(f.Filters) == 0 }

// Validate rejects a filter the API would reject. An empty filter is a
// programming error for every operation that takes one: update, upsert and
// delete all require at least one condition.
func (f RowFilter) Validate() error {
	if len(f.Filters) == 0 {
		return fmt.Errorf("a row filter needs at least one condition")
	}
	if f.Type != "" && !slices.Contains(DataTableFilterTypes, f.Type) {
		return fmt.Errorf("unknown row filter type %q: the API defines %s", f.Type, strings.Join(DataTableFilterTypes, ", "))
	}
	for i, condition := range f.Filters {
		if err := condition.Validate(); err != nil {
			return fmt.Errorf("row filter condition %d: %w", i+1, err)
		}
	}
	return nil
}

// ListRowsOptions are the query parameters of the row list endpoint.
type ListRowsOptions struct {
	ListOptions
	// Filter narrows the rows returned. The zero value sends no filter.
	Filter RowFilter
	// SortBy is "columnName:asc" or "columnName:desc".
	SortBy string
	// Search matches text across all string columns.
	Search string
}

// Validate rejects options the API would reject.
func (o ListRowsOptions) Validate() error {
	if err := validateDataTablePagination(o.ListOptions); err != nil {
		return err
	}
	if err := validateSortBy(o.SortBy); err != nil {
		return err
	}
	if !o.Filter.IsZero() {
		if err := o.Filter.Validate(); err != nil {
			return err
		}
	}
	return validateDataTableText("search text", o.Search, false)
}

func (o ListRowsOptions) apply() (url.Values, error) {
	query := o.ListOptions.Apply(nil)
	if o.SortBy != "" {
		query.Set("sortBy", o.SortBy)
	}
	if o.Search != "" {
		query.Set("search", o.Search)
	}
	if !o.Filter.IsZero() {
		encoded, err := json.Marshal(o.Filter)
		if err != nil {
			return nil, fmt.Errorf("encode row filter: %w", err)
		}
		query.Set("filter", string(encoded))
	}
	return query, nil
}

// DataTableColumnDefinition is one column of a create-table request.
type DataTableColumnDefinition struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CreateDataTableRequest is the write document for table creation. Without a
// project the table is created in the caller's personal project.
type CreateDataTableRequest struct {
	Name      string                      `json:"name"`
	Columns   []DataTableColumnDefinition `json:"columns"`
	ProjectID string                      `json:"projectId,omitempty"`
}

// Validate rejects a malformed creation before transport.
func (r CreateDataTableRequest) Validate() error {
	if err := validateDataTableName(r.Name); err != nil {
		return err
	}
	if len(r.Columns) == 0 {
		return fmt.Errorf("a data table needs at least one column")
	}
	for i, column := range r.Columns {
		if err := validateDataTableText("column name", column.Name, true); err != nil {
			return fmt.Errorf("column %d: %w", i+1, err)
		}
		if !slices.Contains(DataTableCreateColumnTypes, column.Type) {
			return fmt.Errorf("column %d: unknown column type %q: the API defines %s", i+1, column.Type, strings.Join(DataTableCreateColumnTypes, ", "))
		}
	}
	return validateDataTableText("project ID", r.ProjectID, false)
}

// InsertRowsRequest inserts one or more rows. ReturnType selects which of the
// three documented response shapes the API sends back; empty uses the server
// default of "count".
type InsertRowsRequest struct {
	Data       []DataTableRow `json:"data"`
	ReturnType string         `json:"returnType,omitempty"`
}

// Validate rejects a malformed insert before transport.
func (r InsertRowsRequest) Validate() error {
	if len(r.Data) == 0 {
		return fmt.Errorf("at least one row is required")
	}
	for i, row := range r.Data {
		if len(row) == 0 {
			return fmt.Errorf("row %d is empty: give it at least one column value", i+1)
		}
	}
	if r.ReturnType != "" && !slices.Contains(DataTableInsertReturnTypes, r.ReturnType) {
		return fmt.Errorf("unknown return type %q: the API defines %s", r.ReturnType, strings.Join(DataTableInsertReturnTypes, ", "))
	}
	return nil
}

// UpdateRowsRequest updates every row matching Filter. DryRun asks the server
// to report what would change without persisting it.
type UpdateRowsRequest struct {
	Filter     RowFilter    `json:"filter"`
	Data       DataTableRow `json:"data"`
	ReturnData bool         `json:"returnData,omitempty"`
	DryRun     bool         `json:"dryRun,omitempty"`
}

// Validate rejects a malformed update before transport.
func (r UpdateRowsRequest) Validate() error {
	if err := r.Filter.Validate(); err != nil {
		return err
	}
	if len(r.Data) == 0 {
		return fmt.Errorf("at least one column value to write is required")
	}
	return nil
}

// UpsertRowRequest updates the row matching Filter, or inserts one when no row
// matches.
type UpsertRowRequest struct {
	Filter     RowFilter    `json:"filter"`
	Data       DataTableRow `json:"data"`
	ReturnData bool         `json:"returnData,omitempty"`
	DryRun     bool         `json:"dryRun,omitempty"`
}

// Validate rejects a malformed upsert before transport.
func (r UpsertRowRequest) Validate() error {
	if err := r.Filter.Validate(); err != nil {
		return err
	}
	if len(r.Data) == 0 {
		return fmt.Errorf("at least one column value to write is required")
	}
	return nil
}

// DeleteRowsOptions selects the rows to delete. The filter is required by the
// API itself, which is what stops an accidental full delete.
type DeleteRowsOptions struct {
	Filter     RowFilter
	ReturnData bool
	DryRun     bool
}

// Validate rejects a delete the API would reject.
func (o DeleteRowsOptions) Validate() error { return o.Filter.Validate() }

func (o DeleteRowsOptions) apply() (url.Values, error) {
	encoded, err := json.Marshal(o.Filter)
	if err != nil {
		return nil, fmt.Errorf("encode row filter: %w", err)
	}
	query := url.Values{}
	query.Set("filter", string(encoded))
	if o.ReturnData {
		query.Set("returnData", "true")
	}
	if o.DryRun {
		query.Set("dryRun", "true")
	}
	return query, nil
}

// CreateColumnRequest adds one column to an existing table. Index is a pointer
// because zero is a meaningful position and an omitted index appends.
type CreateColumnRequest struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Index *int   `json:"index,omitempty"`
}

// Validate rejects a malformed column before transport.
func (r CreateColumnRequest) Validate() error {
	if err := validateDataTableText("column name", r.Name, true); err != nil {
		return err
	}
	if !slices.Contains(DataTableColumnTypes, r.Type) {
		return fmt.Errorf("unknown column type %q: this endpoint defines %s", r.Type, strings.Join(DataTableColumnTypes, ", "))
	}
	if r.Index != nil && *r.Index < 0 {
		return fmt.Errorf("column index must not be negative, got %d", *r.Index)
	}
	return nil
}

// UpdateColumnRequest renames or reorders one column. At least one of the two
// is required, which is what the contract's anyOf says.
type UpdateColumnRequest struct {
	Name  string `json:"name,omitempty"`
	Index *int   `json:"index,omitempty"`
}

// Validate rejects a malformed column update before transport.
func (r UpdateColumnRequest) Validate() error {
	if r.Name == "" && r.Index == nil {
		return fmt.Errorf("a column update needs a new name or a new index")
	}
	if err := validateDataTableText("column name", r.Name, false); err != nil {
		return err
	}
	if r.Index != nil && *r.Index < 0 {
		return fmt.Errorf("column index must not be negative, got %d", *r.Index)
	}
	return nil
}

// InsertRowsResult preserves whichever shape an insert answered with. The
// contract documents three, and instances vary inside them: a count can arrive
// as {"count":n} or as {"success":true,"insertedRows":n}, and the id mode can
// answer with bare numbers or with objects carrying only an id.
type InsertRowsResult struct {
	// Acknowledged is the success flag that accompanies a count, when sent.
	Acknowledged bool `json:"acknowledged,omitempty"`
	// Count is the number of rows inserted, when the API reported one.
	Count *int `json:"count,omitempty"`
	// IDs are the inserted row IDs, whether they arrived bare or inside rows.
	IDs []int64 `json:"ids,omitempty"`
	// Rows are the inserted rows, when the API returned them.
	Rows []DataTableRow `json:"rows,omitempty"`
}

// UnmarshalJSON decodes every documented and observed insert response shape.
func (r *InsertRowsResult) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	switch {
	case trimmed == "" || trimmed == "null":
		return nil
	case strings.HasPrefix(trimmed, "{"):
		var envelope struct {
			Count        *int  `json:"count"`
			InsertedRows *int  `json:"insertedRows"`
			Success      *bool `json:"success"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			return fmt.Errorf("decode insert result: %w", err)
		}
		r.Count = envelope.Count
		if r.Count == nil {
			r.Count = envelope.InsertedRows
		}
		if envelope.Success != nil {
			r.Acknowledged = *envelope.Success
		} else {
			r.Acknowledged = r.Count != nil
		}
		return nil
	case strings.HasPrefix(trimmed, "["):
		var ids []int64
		if err := json.Unmarshal(data, &ids); err == nil {
			r.IDs = ids
			r.Acknowledged = true
			return nil
		}
		var rows []DataTableRow
		if err := json.Unmarshal(data, &rows); err != nil {
			return fmt.Errorf("decode insert result: %w", err)
		}
		r.Rows = rows
		r.Acknowledged = true
		for _, row := range rows {
			if id, ok := row.RowID(); ok {
				r.IDs = append(r.IDs, id)
			}
		}
		return nil
	default:
		return fmt.Errorf("decode insert result: unexpected response %s", truncateText(trimmed, 120))
	}
}

// RowMutationResult preserves the response of an update, upsert or filtered
// delete: a bare boolean when returnData was false, the affected rows when it
// was true. A dry run returns the same shapes, with each row carrying a
// dryRunState of "before" or "after".
type RowMutationResult struct {
	// Acknowledged is the boolean the API returns without returnData.
	Acknowledged bool `json:"acknowledged"`
	// Rows are the affected rows, when returnData asked for them.
	Rows []DataTableRow `json:"rows,omitempty"`
}

// UnmarshalJSON decodes the boolean, array and single-object shapes.
func (r *RowMutationResult) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	switch {
	case trimmed == "" || trimmed == "null":
		return nil
	case trimmed == "true" || trimmed == "false":
		r.Acknowledged = trimmed == "true"
		return nil
	case strings.HasPrefix(trimmed, "["):
		if err := json.Unmarshal(data, &r.Rows); err != nil {
			return fmt.Errorf("decode row mutation result: %w", err)
		}
		r.Acknowledged = true
		return nil
	case strings.HasPrefix(trimmed, "{"):
		var row DataTableRow
		if err := json.Unmarshal(data, &row); err != nil {
			return fmt.Errorf("decode row mutation result: %w", err)
		}
		r.Rows = []DataTableRow{row}
		r.Acknowledged = true
		return nil
	default:
		return fmt.Errorf("decode row mutation result: unexpected response %s", truncateText(trimmed, 120))
	}
}

// ClearRowsResult is the response of clearing every row of a table.
type ClearRowsResult struct {
	DeletedCount int `json:"deletedCount"`
}

// ListDataTables returns one cursor-paginated page of data tables.
func (c *Client) ListDataTables(ctx context.Context, opts ListDataTablesOptions) (Page[DataTable], error) {
	if err := opts.Validate(); err != nil {
		return Page[DataTable]{}, err
	}
	query, err := opts.apply()
	if err != nil {
		return Page[DataTable]{}, err
	}
	var page Page[DataTable]
	if _, err := c.Do(ctx, Request{Path: DataTablesPath, Query: query}, &page); err != nil {
		return Page[DataTable]{}, err
	}
	return page, nil
}

// CreateDataTable creates one data table with its columns.
func (c *Client) CreateDataTable(ctx context.Context, req CreateDataTableRequest) (*DataTable, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var created DataTable
	if _, err := c.Do(ctx, Request{Method: http.MethodPost, Path: DataTablesPath, Body: req}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// GetDataTable returns one data table with its column definitions.
func (c *Client) GetDataTable(ctx context.Context, id string) (*DataTable, error) {
	if err := validateDataTableID(id); err != nil {
		return nil, err
	}
	var table DataTable
	if _, err := c.Do(ctx, Request{Path: PathJoin("data-tables", id)}, &table); err != nil {
		return nil, err
	}
	return &table, nil
}

// UpdateDataTable renames one data table. Name is the only writable field.
func (c *Client) UpdateDataTable(ctx context.Context, id, name string) (*DataTable, error) {
	if err := validateDataTableID(id); err != nil {
		return nil, err
	}
	if err := validateDataTableName(name); err != nil {
		return nil, err
	}
	var updated DataTable
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPatch,
		Path:   PathJoin("data-tables", id),
		Body: struct {
			Name string `json:"name"`
		}{Name: name},
	}, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// DeleteDataTable deletes one data table and every row in it. The documented
// 204 response has no body.
func (c *Client) DeleteDataTable(ctx context.Context, id string) error {
	if err := validateDataTableID(id); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin("data-tables", id)}, nil)
	return err
}

// ListDataTableRows returns one cursor-paginated page of rows.
func (c *Client) ListDataTableRows(ctx context.Context, tableID string, opts ListRowsOptions) (Page[DataTableRow], error) {
	if err := validateDataTableID(tableID); err != nil {
		return Page[DataTableRow]{}, err
	}
	if err := opts.Validate(); err != nil {
		return Page[DataTableRow]{}, err
	}
	query, err := opts.apply()
	if err != nil {
		return Page[DataTableRow]{}, err
	}
	var page Page[DataTableRow]
	if _, err := c.Do(ctx, Request{Path: PathJoin("data-tables", tableID, "rows"), Query: query}, &page); err != nil {
		return Page[DataTableRow]{}, err
	}
	return page, nil
}

// InsertDataTableRows inserts one or more rows and preserves whichever
// response shape the requested return type produced.
func (c *Client) InsertDataTableRows(ctx context.Context, tableID string, req InsertRowsRequest) (*InsertRowsResult, error) {
	if err := validateDataTableID(tableID); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var result InsertRowsResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("data-tables", tableID, "rows"),
		Body:   req,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateDataTableRows updates every row matching the request filter.
func (c *Client) UpdateDataTableRows(ctx context.Context, tableID string, req UpdateRowsRequest) (*RowMutationResult, error) {
	if err := validateDataTableID(tableID); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var result RowMutationResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPatch,
		Path:   PathJoin("data-tables", tableID, "rows", "update"),
		Body:   req,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpsertDataTableRow updates the row matching the filter or inserts a new one.
func (c *Client) UpsertDataTableRow(ctx context.Context, tableID string, req UpsertRowRequest) (*RowMutationResult, error) {
	if err := validateDataTableID(tableID); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var result RowMutationResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("data-tables", tableID, "rows", "upsert"),
		Body:   req,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ClearDataTableRows deletes every row of a table and keeps its structure.
func (c *Client) ClearDataTableRows(ctx context.Context, tableID string) (*ClearRowsResult, error) {
	if err := validateDataTableID(tableID); err != nil {
		return nil, err
	}
	var result ClearRowsResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("data-tables", tableID, "rows", "clear"),
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteDataTableRows deletes the rows matching the filter. The filter is
// mandatory, and a dry run reports what would go without deleting anything.
func (c *Client) DeleteDataTableRows(ctx context.Context, tableID string, opts DeleteRowsOptions) (*RowMutationResult, error) {
	if err := validateDataTableID(tableID); err != nil {
		return nil, err
	}
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	query, err := opts.apply()
	if err != nil {
		return nil, err
	}
	var result RowMutationResult
	if _, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("data-tables", tableID, "rows", "delete"),
		Query:  query,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListDataTableColumns returns every column of one table. The response is a
// bare array, not a page.
func (c *Client) ListDataTableColumns(ctx context.Context, tableID string) ([]DataTableColumn, error) {
	if err := validateDataTableID(tableID); err != nil {
		return nil, err
	}
	var columns []DataTableColumn
	if _, err := c.Do(ctx, Request{Path: PathJoin("data-tables", tableID, "columns")}, &columns); err != nil {
		return nil, err
	}
	return columns, nil
}

// CreateDataTableColumn adds one column to an existing table.
func (c *Client) CreateDataTableColumn(ctx context.Context, tableID string, req CreateColumnRequest) (*DataTableColumn, error) {
	if err := validateDataTableID(tableID); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var created DataTableColumn
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   PathJoin("data-tables", tableID, "columns"),
		Body:   req,
	}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// UpdateDataTableColumn renames or reorders one column.
func (c *Client) UpdateDataTableColumn(ctx context.Context, tableID, columnID string, req UpdateColumnRequest) (*DataTableColumn, error) {
	if err := validateDataTableID(tableID); err != nil {
		return nil, err
	}
	if err := validateDataTableColumnID(columnID); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var updated DataTableColumn
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPatch,
		Path:   PathJoin("data-tables", tableID, "columns", columnID),
		Body:   req,
	}, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// DeleteDataTableColumn removes one column and the data in it. The documented
// 204 response has no body.
func (c *Client) DeleteDataTableColumn(ctx context.Context, tableID, columnID string) error {
	if err := validateDataTableID(tableID); err != nil {
		return err
	}
	if err := validateDataTableColumnID(columnID); err != nil {
		return err
	}
	_, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   PathJoin("data-tables", tableID, "columns", columnID),
	}, nil)
	return err
}

func validateDataTablePagination(opts ListOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if opts.Limit > maxDataTablePageSize {
		return fmt.Errorf("limit must not exceed %d, got %d", maxDataTablePageSize, opts.Limit)
	}
	return nil
}

func validateSortBy(sortBy string) error {
	if sortBy == "" {
		return nil
	}
	if !sortByPattern.MatchString(sortBy) {
		return fmt.Errorf("sort order %q is not field:asc or field:desc", sortBy)
	}
	return nil
}

func validateDataTableName(name string) error {
	if err := validateDataTableText("name", name, true); err != nil {
		return err
	}
	if len(name) > maxDataTableNameLength {
		return fmt.Errorf("data table name must not exceed %d characters, got %d", maxDataTableNameLength, len(name))
	}
	return nil
}

func validateDataTableID(id string) error { return validateDataTableText("ID", id, true) }

func validateDataTableColumnID(id string) error {
	return validateDataTableText("column ID", id, true)
}

func validateDataTableText(field, value string, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("data table %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("data table %s must not start or end with whitespace", field)
	}
	return nil
}

// JSONValue encodes a command-line value as JSON: a document when the text
// parses as one, a JSON string otherwise. It is what lets a caller write
// age=30, active=true, meta={"a":1} and name=Jane without a type flag.
func JSONValue(text string) json.RawMessage {
	trimmed := strings.TrimSpace(text)
	if trimmed != "" && json.Valid([]byte(trimmed)) && !isBareWord(trimmed) {
		return json.RawMessage(trimmed)
	}
	encoded, err := json.Marshal(text)
	if err != nil {
		// A Go string always encodes, so this cannot happen in practice.
		return json.RawMessage(strconv.Quote(text))
	}
	return encoded
}

// isBareWord reports whether trimmed is valid JSON only because it is a bare
// literal that a caller almost certainly meant as text, such as an unquoted
// word. Numbers, booleans, null, objects, arrays and quoted strings are kept
// as JSON; anything else is text.
func isBareWord(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	switch trimmed[0] {
	case '{', '[', '"', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return false
	}
	return trimmed != "true" && trimmed != "false" && trimmed != "null"
}
