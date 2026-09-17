package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const (
	// dataTableResource is the 'n8n discover' key for data tables.
	dataTableResource = "datatable"
	// dataTableCommand names the group in the API-key requirement error.
	dataTableCommand = "data-table"
	// maxDataTableDocument caps a JSON payload read from a file or stdin.
	maxDataTableDocument = 1 << 20
)

func newDataTableCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data-table",
		Short: "Manage data tables, their rows and their columns",
		Long: "Manage n8n data tables: the table itself, the rows in it, and the columns that\n" +
			"define it. Start with 'list' to find table IDs, then work on rows under the\n" +
			"'row' subgroup and on the schema under the 'column' subgroup.\n\n" +
			"Every command here requires API-key authentication; a bearer token or auth\n" +
			"cookie is refused before any request. Row values are typed: a value that parses\n" +
			"as JSON is sent as JSON (30, true, null, {\"a\":1}), anything else as text.\n" +
			"Deleting a table, clearing its rows, and deleting a column all destroy data and\n" +
			"require confirmation; filtered row updates and deletes support the server-side\n" +
			"--dry-run, which reports what would change without changing it.",
		Example: "  n8n data-table list\n" +
			"  n8n data-table create customers --column email:string --column age:number\n" +
			"  n8n data-table row list TABLE_ID --where status=active\n" +
			"  n8n data-table row insert TABLE_ID --set email=a@example.com --set age=30\n" +
			"  n8n data-table row delete TABLE_ID --where status=archived --dry-run\n" +
			"  n8n data-table column list TABLE_ID",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newDataTableListCommand(opts),
		newDataTableCreateCommand(opts),
		newDataTableGetCommand(opts),
		newDataTableUpdateCommand(opts),
		newDataTableDeleteCommand(opts),
		newDataTableRowCommand(opts),
		newDataTableColumnCommand(opts),
	)
	return cmd
}

// dataTableClient resolves the client and enforces the API-key-only contract
// this whole group is declared with.
func (o Options) dataTableClient(f instanceFlags) (*n8n.Client, config.Resolution, error) {
	client, resolution, err := o.apiClient(f)
	if err != nil {
		return nil, resolution, err
	}
	if err := requireAPIKey(resolution, dataTableCommand); err != nil {
		return nil, resolution, err
	}
	return client, resolution, nil
}

type dataTableListFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	name     string
	sortBy   string
	output   string
}

func newDataTableListCommand(opts Options) *cobra.Command {
	var f dataTableListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List data tables",
		Long: "List the data tables the credential can see, one cursor-paginated page at a\n" +
			"time. Pass a returned cursor to --cursor for the next page, or use --all to\n" +
			"follow every cursor, capped at 10,000 tables.\n\n" +
			"Use this to find the table ID every other command takes. --name is the only\n" +
			"filter the API documents. --sort-by takes field:asc or field:desc, for example\n" +
			"name:asc or size:desc, where size orders by the reported sizeBytes. Requires\n" +
			"dataTable:list and API-key authentication.",
		Example: "  n8n data-table list\n" +
			"  n8n data-table list --name customers\n" +
			"  n8n data-table list --sort-by size:desc --limit 50 --output json\n" +
			"  n8n data-table list --all",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDataTableList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "tables per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 tables)")
	cmd.Flags().StringVar(&f.name, "name", "", "only tables with this name (default: every table)")
	cmd.Flags().StringVar(&f.sortBy, "sort-by", "", "sort order as field:asc or field:desc, e.g. name:asc or size:desc (default: the server's order)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runDataTableList(ctx context.Context, opts Options, f dataTableListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	listOpts := n8n.ListDataTablesOptions{
		ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor},
		Filter:      n8n.DataTableFilter{Name: f.name},
		SortBy:      f.sortBy,
	}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, pagination n8n.ListOptions) (n8n.Page[n8n.DataTable], error) {
		scoped := listOpts
		scoped.ListOptions = pagination
		return client.ListDataTables(ctx, scoped)
	}
	var page n8n.Page[n8n.DataTable]
	if f.all {
		page.Data, err = n8n.Collect(ctx, fetch, listOpts.ListOptions, n8n.DefaultCollectLimit)
	} else {
		page, err = fetch(ctx, listOpts.ListOptions)
	}
	if err != nil {
		return dataTableAPIError(err, resolution, "list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeDataTableListText(opts, resolution, page)
}

func writeDataTableListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.DataTable]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Data tables:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tPROJECT\tCOLUMNS\tSIZE BYTES")
		for _, table := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\n",
				table.ID, table.Name, emptyDash(table.ProjectID), len(table.Columns), table.SizeBytes)
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No data tables were returned. Create one with 'n8n data-table create NAME --column NAME:TYPE'.")
	}
	return nil
}

type dataTableCreateFlags struct {
	instance  instanceFlags
	columns   []string
	projectID string
	output    string
}

func newDataTableCreateCommand(opts Options) *cobra.Command {
	var f dataTableCreateFlags
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a data table",
		Long: "Create one data table with its columns. Pass --column once per column as\n" +
			"NAME:TYPE; at least one is required, and the column order is the order given.\n\n" +
			"Documented types are string, number, boolean, date and json, though instances\n" +
			"can reject json with a 400. Without --project-id the table is created in the\n" +
			"caller's personal project. The system columns id, createdAt and updatedAt are\n" +
			"added by n8n; do not declare them. Add more columns later with\n" +
			"'n8n data-table column create'. Requires dataTable:create.",
		Example: "  n8n data-table create customers --column email:string --column age:number\n" +
			"  n8n data-table create orders --column total:number --column paid:boolean --column placed:date\n" +
			"  n8n data-table create customers --column email:string --project-id PROJECT_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableCreate(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringArrayVar(&f.columns, "column", nil, "column as NAME:TYPE, repeatable and required, e.g. email:string (types: "+strings.Join(n8n.DataTableCreateColumnTypes, ", ")+")")
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "project to create the table in (default: the caller's personal project)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the created table)")
	return cmd
}

func runDataTableCreate(ctx context.Context, opts Options, name string, f dataTableCreateFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	columns, err := dataTableColumnDefinitions(f.columns)
	if err != nil {
		return err
	}
	request := n8n.CreateDataTableRequest{Name: name, Columns: columns, ProjectID: f.projectID}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	table, err := client.CreateDataTable(ctx, request)
	if err != nil {
		return dataTableAPIError(err, resolution, "create")
	}
	return writeDataTableResult(opts, resolution, "Created", table, f.output)
}

// dataTableColumnDefinitions parses the repeatable --column NAME:TYPE flag.
func dataTableColumnDefinitions(values []string) ([]n8n.DataTableColumnDefinition, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("--column is required: pass at least one column as NAME:TYPE, e.g. --column email:string")
	}
	columns := make([]n8n.DataTableColumnDefinition, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		name, columnType, ok := strings.Cut(value, ":")
		if !ok {
			return nil, fmt.Errorf("--column %q is not NAME:TYPE: pass the column name, a colon, then one of %s", value, strings.Join(n8n.DataTableCreateColumnTypes, ", "))
		}
		if seen[name] {
			return nil, fmt.Errorf("--column %q is given twice", name)
		}
		seen[name] = true
		columns = append(columns, n8n.DataTableColumnDefinition{Name: name, Type: columnType})
	}
	return columns, nil
}

type dataTableIDFlags struct {
	instance instanceFlags
	output   string
}

func newDataTableGetCommand(opts Options) *cobra.Command {
	var f dataTableIDFlags
	cmd := &cobra.Command{
		Use:   "get <table-id>",
		Short: "Show one data table and its columns",
		Long: "Show one data table: its name, owning project, storage size, and the columns\n" +
			"that define it, in column order.\n\n" +
			"The reported size is physical storage including indexes. It does not shrink the\n" +
			"moment rows are deleted and can be a few seconds stale, so do not read it as a\n" +
			"row count; use 'n8n data-table row list' for the rows themselves. Requires\n" +
			"dataTable:read.",
		Example: "  n8n data-table get TABLE_ID\n" +
			"  n8n data-table get TABLE_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableGet(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the table object with its columns)")
	return cmd
}

func runDataTableGet(ctx context.Context, opts Options, id string, f dataTableIDFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", id); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	table, err := client.GetDataTable(ctx, id)
	if err != nil {
		return dataTableAPIError(err, resolution, "read")
	}
	return writeDataTableResult(opts, resolution, "Data table", table, f.output)
}

type dataTableUpdateFlags struct {
	instance instanceFlags
	name     string
	output   string
}

func newDataTableUpdateCommand(opts Options) *cobra.Command {
	var f dataTableUpdateFlags
	cmd := &cobra.Command{
		Use:   "update <table-id>",
		Short: "Rename a data table",
		Long: "Rename one data table. The name is the only writable property of a table: its\n" +
			"columns are changed with 'n8n data-table column' and its contents with\n" +
			"'n8n data-table row'.\n\n" +
			"A name already used by another table in the same project comes back as a 409.\n" +
			"Names are limited to 128 characters. Requires dataTable:update.",
		Example: "  n8n data-table update TABLE_ID --name customers-2026\n" +
			"  n8n data-table update TABLE_ID --name archive --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.name, "name", "", "new table name, up to 128 characters (required)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the updated table)")
	return cmd
}

func runDataTableUpdate(ctx context.Context, opts Options, id string, f dataTableUpdateFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", id); err != nil {
		return err
	}
	if strings.TrimSpace(f.name) == "" {
		return fmt.Errorf("--name is required: it is the new name the table is renamed to")
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	table, err := client.UpdateDataTable(ctx, id, f.name)
	if err != nil {
		return dataTableAPIError(err, resolution, "update")
	}
	return writeDataTableResult(opts, resolution, "Updated", table, f.output)
}

type dataTableDeleteFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newDataTableDeleteCommand(opts Options) *cobra.Command {
	var f dataTableDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <table-id>",
		Short: "Permanently delete a data table and every row in it",
		Long: "Permanently delete one data table. Every row it holds and every column it\n" +
			"defines go with it, and workflow nodes that read or write the table start\n" +
			"failing. This cannot be undone and the API offers no export step, so copy\n" +
			"anything worth keeping first with 'n8n data-table row list --all --output json'.\n\n" +
			"To empty a table but keep its structure, use 'n8n data-table row clear' instead.\n" +
			"Interactive runs ask for confirmation. Non-interactive runs require --yes.\n" +
			"Requires dataTable:delete.",
		Example: "  n8n data-table delete TABLE_ID\n" +
			"  n8n data-table delete TABLE_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent deletion of the table and its rows without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (acknowledgement contains the deleted table ID)")
	return cmd
}

func runDataTableDelete(ctx context.Context, opts Options, id string, f dataTableDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", id); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete data table %q from %s? Every row and column in it is deleted with it, and workflows that use the table will fail.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.DeleteDataTable(ctx, id); err != nil {
		return dataTableAPIError(err, resolution, "delete")
	}
	return writeDataTableMutation(opts, resolution, dataTableMutation{Action: "deleted", TableID: id}, f.output)
}

func newDataTableRowCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "row",
		Short: "Read and change the rows of a data table",
		Long: "Work with the contents of one data table: list rows, insert them, update or\n" +
			"upsert the ones matching a filter, delete the ones matching a filter, or clear\n" +
			"the table entirely.\n\n" +
			"Rows are selected with repeatable --where expressions: COLUMN=VALUE compares\n" +
			"with eq, and COLUMN=CONDITION=VALUE picks another condition (" +
			strings.Join(n8n.DataTableRowConditions, ", ") + ").\n" +
			"Several --where expressions combine with and, or with or when --match or is\n" +
			"given; --filter takes the whole documented filter object as JSON instead.\n" +
			"Values are typed: 30, true and null are sent as JSON, other text as a string.\n" +
			"Update, upsert and delete accept --dry-run, which asks the server to report the\n" +
			"before and after rows without writing anything.",
		Example: "  n8n data-table row list TABLE_ID --where status=active --sort-by createdAt:desc\n" +
			"  n8n data-table row insert TABLE_ID --set email=a@example.com --set age=30 --return all\n" +
			"  n8n data-table row update TABLE_ID --where status=pending --set status=done --dry-run\n" +
			"  n8n data-table row upsert TABLE_ID --where email=a@example.com --set name=Jane\n" +
			"  n8n data-table row delete TABLE_ID --where status=archived --yes\n" +
			"  n8n data-table row clear TABLE_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newDataTableRowListCommand(opts),
		newDataTableRowInsertCommand(opts),
		newDataTableRowUpdateCommand(opts),
		newDataTableRowUpsertCommand(opts),
		newDataTableRowClearCommand(opts),
		newDataTableRowDeleteCommand(opts),
	)
	return cmd
}

// rowFilterFlags are the row-selection flags shared by list, update, upsert
// and delete.
type rowFilterFlags struct {
	where  []string
	match  string
	filter string
}

func (f *rowFilterFlags) register(cmd *cobra.Command, required bool) {
	suffix := ""
	if required {
		suffix = " (required unless --filter is given)"
	}
	cmd.Flags().StringArrayVar(&f.where, "where", nil, "row condition as COLUMN=VALUE or COLUMN=CONDITION=VALUE, repeatable, conditions: "+strings.Join(n8n.DataTableRowConditions, ", ")+suffix)
	cmd.Flags().StringVar(&f.match, "match", "and", "how several --where expressions combine: and or or")
	cmd.Flags().StringVar(&f.filter, "filter", "", "whole filter object as JSON, e.g. '{\"type\":\"and\",\"filters\":[{\"columnName\":\"status\",\"condition\":\"eq\",\"value\":\"active\"}]}' (cannot combine with --where)")
}

// filter builds the row filter from --where or --filter, which are mutually
// exclusive. required says whether an empty selection is an error.
func (f rowFilterFlags) build(required bool) (n8n.RowFilter, error) {
	hasWhere, hasFilter := len(f.where) > 0, strings.TrimSpace(f.filter) != ""
	switch {
	case hasWhere && hasFilter:
		return n8n.RowFilter{}, fmt.Errorf("--where and --filter cannot be used together: select the rows one way")
	case !hasWhere && !hasFilter:
		if required {
			return n8n.RowFilter{}, fmt.Errorf("--where or --filter is required: this command refuses to touch every row of the table")
		}
		return n8n.RowFilter{}, nil
	case hasFilter:
		var filter n8n.RowFilter
		if err := decodeDataTableJSON(strings.NewReader(f.filter), &filter); err != nil {
			return n8n.RowFilter{}, fmt.Errorf("--filter: %w", err)
		}
		if err := filter.Validate(); err != nil {
			return n8n.RowFilter{}, err
		}
		return filter, nil
	}
	if f.match != "and" && f.match != "or" {
		return n8n.RowFilter{}, fmt.Errorf("--match %q is neither and nor or", f.match)
	}
	filter := n8n.RowFilter{Type: f.match}
	for _, expression := range f.where {
		condition, err := parseRowCondition(expression)
		if err != nil {
			return n8n.RowFilter{}, err
		}
		filter.Filters = append(filter.Filters, condition)
	}
	if err := filter.Validate(); err != nil {
		return n8n.RowFilter{}, err
	}
	return filter, nil
}

// parseRowCondition reads COLUMN=VALUE or COLUMN=CONDITION=VALUE. A three-part
// expression whose middle is not a known condition is treated as a value that
// happens to contain an equals sign, so status=a=b compares status with "a=b".
func parseRowCondition(expression string) (n8n.RowCondition, error) {
	parts := strings.SplitN(expression, "=", 3)
	if len(parts) < 2 {
		return n8n.RowCondition{}, fmt.Errorf("--where %q is not COLUMN=VALUE or COLUMN=CONDITION=VALUE", expression)
	}
	column, condition, value := parts[0], "eq", parts[1]
	if len(parts) == 3 {
		if slices.Contains(n8n.DataTableRowConditions, parts[1]) {
			condition, value = parts[1], parts[2]
		} else {
			value = parts[1] + "=" + parts[2]
		}
	}
	if strings.TrimSpace(column) == "" {
		return n8n.RowCondition{}, fmt.Errorf("--where %q has no column name", expression)
	}
	return n8n.RowCondition{ColumnName: column, Condition: condition, Value: n8n.JSONValue(value)}, nil
}

// rowValues parses repeatable --set COLUMN=VALUE pairs into one row.
func rowValues(values []string) (n8n.DataTableRow, error) {
	row := n8n.DataTableRow{}
	for _, pair := range values {
		column, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("--set %q is not COLUMN=VALUE", pair)
		}
		if strings.TrimSpace(column) == "" {
			return nil, fmt.Errorf("--set %q has no column name", pair)
		}
		if _, exists := row[column]; exists {
			return nil, fmt.Errorf("--set %q is given twice", column)
		}
		row[column] = n8n.JSONValue(value)
	}
	return row, nil
}

type dataTableRowListFlags struct {
	instance instanceFlags
	filter   rowFilterFlags
	limit    int
	cursor   string
	all      bool
	sortBy   string
	search   string
	output   string
}

func newDataTableRowListCommand(opts Options) *cobra.Command {
	var f dataTableRowListFlags
	cmd := &cobra.Command{
		Use:   "list <table-id>",
		Short: "List the rows of a data table",
		Long: "List rows from one data table, one cursor-paginated page at a time. Pass a\n" +
			"returned cursor to --cursor, or use --all to follow every cursor, capped at\n" +
			"10,000 rows.\n\n" +
			"Narrow the rows with repeatable --where expressions (combined with --match and\n" +
			"or or), or with the whole filter object as JSON through --filter. --search\n" +
			"matches text across every string column, and --sort-by takes columnName:asc or\n" +
			"columnName:desc. Text output prints the system columns first, then the\n" +
			"user-defined ones; --output json is the faithful copy of the row data and the\n" +
			"right input for a backup. Requires dataTableRow:read.",
		Example: "  n8n data-table row list TABLE_ID\n" +
			"  n8n data-table row list TABLE_ID --where status=active --limit 50\n" +
			"  n8n data-table row list TABLE_ID --where age=gte=30 --sort-by age:desc\n" +
			"  n8n data-table row list TABLE_ID --search example.com\n" +
			"  n8n data-table row list TABLE_ID --all --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableRowList(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	f.filter.register(cmd, false)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "rows per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 rows)")
	cmd.Flags().StringVar(&f.sortBy, "sort-by", "", "sort order as columnName:asc or columnName:desc (default: the server's order)")
	cmd.Flags().StringVar(&f.search, "search", "", "match this text across every string column (default: no search)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runDataTableRowList(ctx context.Context, opts Options, tableID string, f dataTableRowListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", tableID); err != nil {
		return err
	}
	filter, err := f.filter.build(false)
	if err != nil {
		return err
	}
	listOpts := n8n.ListRowsOptions{
		ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor},
		Filter:      filter,
		SortBy:      f.sortBy,
		Search:      f.search,
	}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, pagination n8n.ListOptions) (n8n.Page[n8n.DataTableRow], error) {
		scoped := listOpts
		scoped.ListOptions = pagination
		return client.ListDataTableRows(ctx, tableID, scoped)
	}
	var page n8n.Page[n8n.DataTableRow]
	if f.all {
		page.Data, err = n8n.Collect(ctx, fetch, listOpts.ListOptions, n8n.DefaultCollectLimit)
	} else {
		page, err = fetch(ctx, listOpts.ListOptions)
	}
	if err != nil {
		return dataTableAPIError(err, resolution, "row list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Data table:\t%s\n", tableID)
	fmt.Fprintf(tw, "Rows:\t%d\n", len(page.Data))
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) > 0 {
		fmt.Fprintln(opts.Streams.Out)
		if err := writeDataTableRows(opts.Streams.Out, page.Data); err != nil {
			return err
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(opts.Streams.Out, "\nNext cursor: %s\n", page.NextCursor)
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No rows were returned. Insert one with 'n8n data-table row insert TABLE_ID --set COLUMN=VALUE'.")
	}
	return nil
}

// writeDataTableRows renders rows as a table whose columns are the union of
// the columns present, system columns first.
func writeDataTableRows(w io.Writer, rows []n8n.DataTableRow) error {
	var columns []string
	for _, row := range rows {
		for _, name := range row.Columns() {
			if !slices.Contains(columns, name) {
				columns = append(columns, name)
			}
		}
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := make([]string, len(columns))
	for i, name := range columns {
		header[i] = strings.ToUpper(name)
	}
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, row := range rows {
		cells := make([]string, len(columns))
		for i, name := range columns {
			cells[i] = emptyDash(row.Text(name))
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

type dataTableRowInsertFlags struct {
	instance   instanceFlags
	set        []string
	input      string
	returnType string
	output     string
}

func newDataTableRowInsertCommand(opts Options) *cobra.Command {
	var f dataTableRowInsertFlags
	cmd := &cobra.Command{
		Use:   "insert <table-id>",
		Short: "Insert rows into a data table",
		Long: "Insert one or more rows. Pass one row with repeatable --set COLUMN=VALUE, or a\n" +
			"JSON array of row objects (or one object) with --input FILE or --input -. The\n" +
			"two ways are mutually exclusive and one is required.\n\n" +
			"Values are typed by their JSON form: age=30 is a number, active=true a boolean,\n" +
			"note=null a null, and anything that is not valid JSON is sent as text. Columns\n" +
			"must already exist; add them with 'n8n data-table column create'. --return\n" +
			"selects what comes back: count, the inserted ids, or the full rows. The whole\n" +
			"batch is one request, so the API accepts or rejects it as a whole. Requires\n" +
			"dataTableRow:create.",
		Example: "  n8n data-table row insert TABLE_ID --set email=a@example.com --set age=30\n" +
			"  n8n data-table row insert TABLE_ID --set email=b@example.com --return all --output json\n" +
			"  n8n data-table row insert TABLE_ID --input rows.json --return id\n" +
			"  printf '%s\\n' '[{\"email\":\"c@example.com\"}]' | n8n data-table row insert TABLE_ID --input -",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableRowInsert(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringArrayVar(&f.set, "set", nil, "column value as COLUMN=VALUE for one row, repeatable (cannot combine with --input)")
	cmd.Flags().StringVar(&f.input, "input", "", "row JSON array or object, from a file path or - for stdin (cannot combine with --set; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.returnType, "return", "", "what the API returns: count, id or all (default: the server default of count)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON preserves the count, ids and rows the API sent)")
	return cmd
}

func runDataTableRowInsert(ctx context.Context, opts Options, tableID string, f dataTableRowInsertFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", tableID); err != nil {
		return err
	}
	rows, err := insertRows(opts, f)
	if err != nil {
		return err
	}
	request := n8n.InsertRowsRequest{Data: rows, ReturnType: f.returnType}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	result, err := client.InsertDataTableRows(ctx, tableID, request)
	if err != nil {
		return dataTableAPIError(err, resolution, "row insert")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Action:\trows inserted\n")
	fmt.Fprintf(tw, "Data table:\t%s\n", tableID)
	if result.Count != nil {
		fmt.Fprintf(tw, "Inserted:\t%d\n", *result.Count)
	} else {
		fmt.Fprintf(tw, "Inserted:\t%d\n", max(len(result.Rows), len(result.IDs)))
	}
	if len(result.IDs) > 0 {
		fmt.Fprintf(tw, "Row IDs:\t%s\n", joinInts(result.IDs))
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(result.Rows) > 0 {
		fmt.Fprintln(opts.Streams.Out)
		return writeDataTableRows(opts.Streams.Out, result.Rows)
	}
	return nil
}

// insertRows builds the rows to insert from --set pairs or --input JSON.
func insertRows(opts Options, f dataTableRowInsertFlags) ([]n8n.DataTableRow, error) {
	hasSet, hasInput := len(f.set) > 0, strings.TrimSpace(f.input) != ""
	switch {
	case hasSet && hasInput:
		return nil, fmt.Errorf("--set and --input cannot be used together: pass the rows one way")
	case !hasSet && !hasInput:
		return nil, fmt.Errorf("--set or --input is required: pass --set COLUMN=VALUE, or row JSON with --input")
	case hasSet:
		row, err := rowValues(f.set)
		if err != nil {
			return nil, err
		}
		return []n8n.DataTableRow{row}, nil
	}
	raw, err := readDataTableInput(opts, f.input)
	if err != nil {
		return nil, err
	}
	if trimmed := strings.TrimSpace(string(raw)); strings.HasPrefix(trimmed, "{") {
		var row n8n.DataTableRow
		if err := decodeDataTableJSON(bytes.NewReader(raw), &row); err != nil {
			return nil, err
		}
		return []n8n.DataTableRow{row}, nil
	}
	var rows []n8n.DataTableRow
	if err := decodeDataTableJSON(bytes.NewReader(raw), &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// rowWriteFlags are the flags shared by the filtered update and upsert.
type rowWriteFlags struct {
	instance   instanceFlags
	filter     rowFilterFlags
	set        []string
	input      string
	returnData bool
	dryRun     bool
	output     string
}

func (f *rowWriteFlags) register(cmd *cobra.Command) {
	f.instance.register(cmd)
	f.filter.register(cmd, true)
	cmd.Flags().StringArrayVar(&f.set, "set", nil, "column value to write as COLUMN=VALUE, repeatable (cannot combine with --input)")
	cmd.Flags().StringVar(&f.input, "input", "", "column values as a JSON object, from a file path or - for stdin (cannot combine with --set; maximum 1 MiB)")
	cmd.Flags().BoolVar(&f.returnData, "return-data", false, "ask the API for the affected rows instead of a bare acknowledgement")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "ask the server to report the change without persisting it (implies --return-data)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON contains the acknowledgement and any returned rows)")
}

// writeRow builds the column values from --set pairs or --input JSON.
func (f rowWriteFlags) writeRow(opts Options) (n8n.DataTableRow, error) {
	hasSet, hasInput := len(f.set) > 0, strings.TrimSpace(f.input) != ""
	switch {
	case hasSet && hasInput:
		return nil, fmt.Errorf("--set and --input cannot be used together: pass the values one way")
	case !hasSet && !hasInput:
		return nil, fmt.Errorf("--set or --input is required: pass --set COLUMN=VALUE, or a JSON object with --input")
	case hasSet:
		return rowValues(f.set)
	}
	raw, err := readDataTableInput(opts, f.input)
	if err != nil {
		return nil, err
	}
	var row n8n.DataTableRow
	if err := decodeDataTableJSON(bytes.NewReader(raw), &row); err != nil {
		return nil, err
	}
	return row, nil
}

func newDataTableRowUpdateCommand(opts Options) *cobra.Command {
	var f rowWriteFlags
	cmd := &cobra.Command{
		Use:   "update <table-id>",
		Short: "Update every row matching a filter",
		Long: "Write column values into every row matching the filter. Select the rows with\n" +
			"repeatable --where expressions or with --filter, and give the new values with\n" +
			"repeatable --set COLUMN=VALUE or a JSON object through --input. A selection is\n" +
			"required: this command will not rewrite a whole table by accident.\n\n" +
			"Run it with --dry-run first on anything important. A dry run asks the server\n" +
			"for the rows it would touch and returns each one twice, marked dryRunState\n" +
			"before and after, without writing anything. Rows that do not exist are not\n" +
			"created; use 'n8n data-table row upsert' for that. Requires\n" +
			"dataTableRow:update.",
		Example: "  n8n data-table row update TABLE_ID --where status=pending --set status=done --dry-run\n" +
			"  n8n data-table row update TABLE_ID --where status=pending --set status=done\n" +
			"  n8n data-table row update TABLE_ID --where age=gte=65 --set discount=true --return-data\n" +
			"  n8n data-table row update TABLE_ID --filter '{\"filters\":[{\"columnName\":\"id\",\"condition\":\"eq\",\"value\":1}]}' --input values.json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableRowUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	f.register(cmd)
	return cmd
}

func runDataTableRowUpdate(ctx context.Context, opts Options, tableID string, f rowWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", tableID); err != nil {
		return err
	}
	filter, err := f.filter.build(true)
	if err != nil {
		return err
	}
	data, err := f.writeRow(opts)
	if err != nil {
		return err
	}
	request := n8n.UpdateRowsRequest{
		Filter:     filter,
		Data:       data,
		ReturnData: f.returnData || f.dryRun,
		DryRun:     f.dryRun,
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	result, err := client.UpdateDataTableRows(ctx, tableID, request)
	if err != nil {
		return dataTableAPIError(err, resolution, "row update")
	}
	return writeRowMutation(opts, resolution, tableID, "rows updated", f.dryRun, result, f.output)
}

func newDataTableRowUpsertCommand(opts Options) *cobra.Command {
	var f rowWriteFlags
	cmd := &cobra.Command{
		Use:   "upsert <table-id>",
		Short: "Update the row matching a filter, or insert it",
		Long: "Update the row matching the filter, or insert a new row when nothing matches.\n" +
			"Select with repeatable --where expressions or --filter, and give the values\n" +
			"with repeatable --set COLUMN=VALUE or a JSON object through --input.\n\n" +
			"The inserted row is built from the values given, not from the filter, so\n" +
			"include the matched column in --set when the new row should carry it. Use\n" +
			"--dry-run to see what would happen first. Requires dataTableRow:upsert.",
		Example: "  n8n data-table row upsert TABLE_ID --where email=a@example.com --set email=a@example.com --set name=Jane\n" +
			"  n8n data-table row upsert TABLE_ID --where email=a@example.com --set name=Jane --return-data\n" +
			"  n8n data-table row upsert TABLE_ID --where email=a@example.com --set name=Jane --dry-run --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableRowUpsert(cmd.Context(), opts, args[0], f)
		},
	}
	f.register(cmd)
	return cmd
}

func runDataTableRowUpsert(ctx context.Context, opts Options, tableID string, f rowWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", tableID); err != nil {
		return err
	}
	filter, err := f.filter.build(true)
	if err != nil {
		return err
	}
	data, err := f.writeRow(opts)
	if err != nil {
		return err
	}
	request := n8n.UpsertRowRequest{
		Filter:     filter,
		Data:       data,
		ReturnData: f.returnData || f.dryRun,
		DryRun:     f.dryRun,
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	result, err := client.UpsertDataTableRow(ctx, tableID, request)
	if err != nil {
		return dataTableAPIError(err, resolution, "row upsert")
	}
	return writeRowMutation(opts, resolution, tableID, "row upserted", f.dryRun, result, f.output)
}

type dataTableRowClearFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newDataTableRowClearCommand(opts Options) *cobra.Command {
	var f dataTableRowClearFlags
	cmd := &cobra.Command{
		Use:   "clear <table-id>",
		Short: "Permanently delete every row of a data table",
		Long: "Permanently delete every row of one data table. The table, its columns and its\n" +
			"ID stay, so workflows that use it keep working against an empty table. There is\n" +
			"no filter and no undo; the response reports how many rows were deleted.\n\n" +
			"Copy the rows first with 'n8n data-table row list --all --output json' if they\n" +
			"matter, or delete a subset with 'n8n data-table row delete --where ...', which\n" +
			"supports a dry run. Interactive runs ask for confirmation. Non-interactive runs\n" +
			"require --yes. Requires dataTableRow:delete.",
		Example: "  n8n data-table row clear TABLE_ID\n" +
			"  n8n data-table row clear TABLE_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableRowClear(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm deletion of every row without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON contains the deleted row count)")
	return cmd
}

func runDataTableRowClear(ctx context.Context, opts Options, tableID string, f dataTableRowClearFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", tableID); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete every row of data table %q on %s? The table and its columns stay; the data does not, and it cannot be recovered.", tableID, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	result, err := client.ClearDataTableRows(ctx, tableID)
	if err != nil {
		return dataTableAPIError(err, resolution, "row clear")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Action:\trows cleared")
	fmt.Fprintf(tw, "Data table:\t%s\n", tableID)
	fmt.Fprintf(tw, "Deleted rows:\t%d\n", result.DeletedCount)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type dataTableRowDeleteFlags struct {
	instance   instanceFlags
	filter     rowFilterFlags
	returnData bool
	dryRun     bool
	yes        bool
	output     string
}

func newDataTableRowDeleteCommand(opts Options) *cobra.Command {
	var f dataTableRowDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <table-id>",
		Short: "Permanently delete the rows matching a filter",
		Long: "Permanently delete every row matching the filter. Select the rows with\n" +
			"repeatable --where expressions or with --filter; the API itself requires a\n" +
			"filter, which is what keeps this from emptying a table by accident. Use\n" +
			"'n8n data-table row clear' when emptying it is the intent.\n\n" +
			"--dry-run asks the server which rows would go and deletes nothing, so it needs\n" +
			"no confirmation; a real delete asks for one, or takes --yes. Deleted rows\n" +
			"cannot be recovered. Requires dataTableRow:delete.",
		Example: "  n8n data-table row delete TABLE_ID --where status=archived --dry-run\n" +
			"  n8n data-table row delete TABLE_ID --where status=archived --yes\n" +
			"  n8n data-table row delete TABLE_ID --where age=lt=18 --match and --where status=inactive --yes\n" +
			"  n8n data-table row delete TABLE_ID --where id=1 --return-data --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableRowDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	f.filter.register(cmd, true)
	cmd.Flags().BoolVar(&f.returnData, "return-data", false, "ask the API for the deleted rows instead of a bare acknowledgement")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "report which rows would be deleted without deleting them (implies --return-data; no confirmation needed)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent deletion of the matching rows without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON contains the acknowledgement and any returned rows)")
	return cmd
}

func runDataTableRowDelete(ctx context.Context, opts Options, tableID string, f dataTableRowDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", tableID); err != nil {
		return err
	}
	filter, err := f.filter.build(true)
	if err != nil {
		return err
	}
	deleteOpts := n8n.DeleteRowsOptions{
		Filter:     filter,
		ReturnData: f.returnData || f.dryRun,
		DryRun:     f.dryRun,
	}
	if err := deleteOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	if !f.dryRun {
		question := fmt.Sprintf("Permanently delete every row of data table %q on %s matching %s? Deleted rows cannot be recovered; --dry-run reports them without deleting.",
			tableID, resolution.URL, describeRowFilter(filter))
		if err := opts.confirmer(f.yes).Confirm(question); err != nil {
			return err
		}
	}
	result, err := client.DeleteDataTableRows(ctx, tableID, deleteOpts)
	if err != nil {
		return dataTableAPIError(err, resolution, "row delete")
	}
	return writeRowMutation(opts, resolution, tableID, "rows deleted", f.dryRun, result, f.output)
}

// describeRowFilter renders the filter for the confirmation prompt, so the
// person answering sees which rows are at stake.
func describeRowFilter(filter n8n.RowFilter) string {
	parts := make([]string, 0, len(filter.Filters))
	for _, condition := range filter.Filters {
		parts = append(parts, fmt.Sprintf("%s %s %s", condition.ColumnName, condition.Condition, string(condition.Value)))
	}
	joiner := " and "
	if filter.Type == "or" {
		joiner = " or "
	}
	return strings.Join(parts, joiner)
}

func writeRowMutation(opts Options, resolution config.Resolution, tableID, action string, dryRun bool, result *n8n.RowMutationResult, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	if dryRun {
		fmt.Fprintf(tw, "Action:\t%s (dry run, nothing was changed)\n", action)
	} else {
		fmt.Fprintf(tw, "Action:\t%s\n", action)
	}
	fmt.Fprintf(tw, "Data table:\t%s\n", tableID)
	fmt.Fprintf(tw, "Acknowledged:\t%t\n", result.Acknowledged)
	if len(result.Rows) > 0 {
		fmt.Fprintf(tw, "Rows returned:\t%d\n", len(result.Rows))
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(result.Rows) > 0 {
		fmt.Fprintln(opts.Streams.Out)
		return writeDataTableRows(opts.Streams.Out, result.Rows)
	}
	return nil
}

func newDataTableColumnCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "column",
		Short: "Manage the columns of a data table",
		Long: "Manage the schema of one data table: list its columns, add one, rename or\n" +
			"reorder one, or delete one.\n\n" +
			"Columns have a type fixed at creation (" + strings.Join(n8n.DataTableColumnTypes, ", ") + ")\n" +
			"and a zero-based position. The system columns id, createdAt and updatedAt are\n" +
			"managed by n8n and are not listed here. Deleting a column deletes the data in\n" +
			"it for every row.",
		Example: "  n8n data-table column list TABLE_ID\n" +
			"  n8n data-table column create TABLE_ID status string\n" +
			"  n8n data-table column update TABLE_ID COLUMN_ID --name state --index 0\n" +
			"  n8n data-table column delete TABLE_ID COLUMN_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newDataTableColumnListCommand(opts),
		newDataTableColumnCreateCommand(opts),
		newDataTableColumnUpdateCommand(opts),
		newDataTableColumnDeleteCommand(opts),
	)
	return cmd
}

func newDataTableColumnListCommand(opts Options) *cobra.Command {
	var f dataTableIDFlags
	cmd := &cobra.Command{
		Use:   "list <table-id>",
		Short: "List the columns of a data table",
		Long: "List every user-defined column of one data table with its ID, type and\n" +
			"position. The response is a plain array, not a page, so there is no pagination\n" +
			"here.\n\n" +
			"Use the column IDs with 'n8n data-table column update' and\n" +
			"'n8n data-table column delete', and the column names with the row commands.\n" +
			"Requires dataTableColumn:read.",
		Example: "  n8n data-table column list TABLE_ID\n" +
			"  n8n data-table column list TABLE_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableColumnList(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the column array)")
	return cmd
}

func runDataTableColumnList(ctx context.Context, opts Options, tableID string, f dataTableIDFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", tableID); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	columns, err := client.ListDataTableColumns(ctx, tableID)
	if err != nil {
		return dataTableAPIError(err, resolution, "column list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, columns)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Data table:\t%s\n", tableID)
	fmt.Fprintf(tw, "Columns:\t%d\n", len(columns))
	if len(columns) > 0 {
		fmt.Fprintln(tw, "\nINDEX\tID\tNAME\tTYPE")
		for _, column := range columns {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", column.Index, emptyDash(column.ID), column.Name, emptyDash(column.Type))
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(columns) == 0 {
		fmt.Fprintln(opts.Streams.Err, "This table has no user-defined columns. Add one with 'n8n data-table column create TABLE_ID NAME TYPE'.")
	}
	return nil
}

type dataTableColumnWriteFlags struct {
	instance instanceFlags
	name     string
	index    int
	output   string
}

func newDataTableColumnCreateCommand(opts Options) *cobra.Command {
	var f dataTableColumnWriteFlags
	cmd := &cobra.Command{
		Use:   "create <table-id> <name> <type>",
		Short: "Add a column to a data table",
		Long: "Add one column to an existing data table. The type is one of " +
			strings.Join(n8n.DataTableColumnTypes, ", ") + "\n" +
			"and cannot be changed afterwards; recreate the column to change it.\n\n" +
			"Without --index the column is appended after the existing ones. Existing rows\n" +
			"get an empty value in the new column. A name already used in the table comes\n" +
			"back as a 409. Requires dataTableColumn:create.",
		Example: "  n8n data-table column create TABLE_ID status string\n" +
			"  n8n data-table column create TABLE_ID priority number --index 0\n" +
			"  n8n data-table column create TABLE_ID signed_at date --output json",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableColumnCreate(cmd.Context(), opts, args[0], args[1], args[2], cmd.Flags().Changed("index"), f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.index, "index", 0, "zero-based position for the column (default: appended after the existing columns)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the created column)")
	return cmd
}

func runDataTableColumnCreate(ctx context.Context, opts Options, tableID, name, columnType string, indexSet bool, f dataTableColumnWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", tableID); err != nil {
		return err
	}
	request := n8n.CreateColumnRequest{Name: name, Type: columnType}
	if indexSet {
		request.Index = &f.index
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	column, err := client.CreateDataTableColumn(ctx, tableID, request)
	if err != nil {
		return dataTableAPIError(err, resolution, "column create")
	}
	return writeDataTableColumn(opts, resolution, "Created", tableID, column, f.output)
}

func newDataTableColumnUpdateCommand(opts Options) *cobra.Command {
	var f dataTableColumnWriteFlags
	cmd := &cobra.Command{
		Use:   "update <table-id> <column-id>",
		Short: "Rename or reorder a column",
		Long: "Rename one column, move it to another position, or both. At least one of --name\n" +
			"and --index is required; the type of a column cannot be changed.\n\n" +
			"Renaming a column changes the key the row commands use for it, so update any\n" +
			"saved --set, --where and --input documents afterwards, and check workflows that\n" +
			"read the table. --index is zero-based and shifts the other columns around it.\n" +
			"Find column IDs with 'n8n data-table column list'. Requires\n" +
			"dataTableColumn:update.",
		Example: "  n8n data-table column update TABLE_ID COLUMN_ID --name state\n" +
			"  n8n data-table column update TABLE_ID COLUMN_ID --index 0\n" +
			"  n8n data-table column update TABLE_ID COLUMN_ID --name state --index 1 --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableColumnUpdate(cmd.Context(), opts, args[0], args[1], cmd.Flags().Changed("index"), f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.name, "name", "", "new column name (required unless --index is given)")
	cmd.Flags().IntVar(&f.index, "index", 0, "new zero-based position (required unless --name is given)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the updated column)")
	return cmd
}

func runDataTableColumnUpdate(ctx context.Context, opts Options, tableID, columnID string, indexSet bool, f dataTableColumnWriteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", tableID); err != nil {
		return err
	}
	if err := validateDataTableArgument("column ID", columnID); err != nil {
		return err
	}
	request := n8n.UpdateColumnRequest{Name: f.name}
	if indexSet {
		request.Index = &f.index
	}
	if request.Name == "" && request.Index == nil {
		return fmt.Errorf("--name or --index is required: one renames the column, the other moves it")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	column, err := client.UpdateDataTableColumn(ctx, tableID, columnID, request)
	if err != nil {
		return dataTableAPIError(err, resolution, "column update")
	}
	return writeDataTableColumn(opts, resolution, "Updated", tableID, column, f.output)
}

type dataTableColumnDeleteFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newDataTableColumnDeleteCommand(opts Options) *cobra.Command {
	var f dataTableColumnDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <table-id> <column-id>",
		Short: "Permanently delete a column and its data",
		Long: "Permanently delete one column from a data table. The value that column holds is\n" +
			"deleted in every row, not only in the schema, and it cannot be recovered. The\n" +
			"remaining columns close the gap in the index order.\n\n" +
			"Workflows that read or write this column start failing, so check them first.\n" +
			"Copy the data with 'n8n data-table row list --all --output json' if it matters.\n" +
			"Interactive runs ask for confirmation. Non-interactive runs require --yes.\n" +
			"Requires dataTableColumn:delete.",
		Example: "  n8n data-table column delete TABLE_ID COLUMN_ID\n" +
			"  n8n data-table column delete TABLE_ID COLUMN_ID --yes --output json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDataTableColumnDelete(cmd.Context(), opts, args[0], args[1], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent deletion of the column and its data without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (acknowledgement contains the table and column)")
	return cmd
}

func runDataTableColumnDelete(ctx context.Context, opts Options, tableID, columnID string, f dataTableColumnDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateDataTableArgument("data table ID", tableID); err != nil {
		return err
	}
	if err := validateDataTableArgument("column ID", columnID); err != nil {
		return err
	}
	client, resolution, err := opts.dataTableClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete column %q from data table %q on %s? The value it holds is deleted in every row and cannot be recovered.", columnID, tableID, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.DeleteDataTableColumn(ctx, tableID, columnID); err != nil {
		return dataTableAPIError(err, resolution, "column delete")
	}
	return writeDataTableMutation(opts, resolution, dataTableMutation{
		Action:   "column deleted",
		TableID:  tableID,
		ColumnID: columnID,
	}, f.output)
}

func writeDataTableResult(opts Options, resolution config.Resolution, action string, table *n8n.DataTable, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, table)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, table.Name)
	fmt.Fprintf(tw, "ID:\t%s\n", emptyDash(table.ID))
	fmt.Fprintf(tw, "Project:\t%s\n", emptyDash(table.ProjectID))
	fmt.Fprintf(tw, "Size bytes:\t%d\n", table.SizeBytes)
	fmt.Fprintf(tw, "Created:\t%s\n", emptyDash(table.CreatedAt))
	fmt.Fprintf(tw, "Updated:\t%s\n", emptyDash(table.UpdatedAt))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if len(table.Columns) > 0 {
		fmt.Fprintln(tw, "\nINDEX\tID\tNAME\tTYPE")
		for _, column := range table.Columns {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", column.Index, emptyDash(column.ID), column.Name, emptyDash(column.Type))
		}
	}
	return tw.Flush()
}

func writeDataTableColumn(opts Options, resolution config.Resolution, action, tableID string, column *n8n.DataTableColumn, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, column)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, column.Name)
	fmt.Fprintf(tw, "ID:\t%s\n", emptyDash(column.ID))
	fmt.Fprintf(tw, "Data table:\t%s\n", tableID)
	fmt.Fprintf(tw, "Type:\t%s\n", emptyDash(column.Type))
	fmt.Fprintf(tw, "Index:\t%d\n", column.Index)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

type dataTableMutation struct {
	Action   string `json:"action"`
	TableID  string `json:"dataTableId"`
	ColumnID string `json:"columnId,omitempty"`
}

func writeDataTableMutation(opts Options, resolution config.Resolution, result dataTableMutation, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Action:\t%s\n", result.Action)
	fmt.Fprintf(tw, "Data table:\t%s\n", result.TableID)
	if result.ColumnID != "" {
		fmt.Fprintf(tw, "Column:\t%s\n", result.ColumnID)
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

// readDataTableInput reads a JSON payload from a file or stdin, capped so a
// wrong path cannot pull an arbitrarily large file into memory.
func readDataTableInput(opts Options, path string) ([]byte, error) {
	var reader io.Reader = opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open data table input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxDataTableDocument+1))
	if err != nil {
		return nil, fmt.Errorf("read data table JSON: %w", err)
	}
	if len(raw) > maxDataTableDocument {
		return nil, fmt.Errorf("data table JSON exceeds %d bytes", maxDataTableDocument)
	}
	return raw, nil
}

// decodeDataTableJSON decodes one strict JSON document and refuses a second.
func decodeDataTableJSON(r io.Reader, dst any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode data table JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode data table JSON: multiple documents are not allowed")
		}
		return fmt.Errorf("decode data table JSON: %w", err)
	}
	return nil
}

func joinInts(values []int64) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprint(value)
	}
	return strings.Join(parts, ", ")
}

func validateDataTableArgument(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not start or end with whitespace", field)
	}
	return nil
}

// dataTableAPIError explains the denials this resource produces. Data tables
// are API-key only and scoped per sub-resource, so a bare 403 does not say
// which scope is missing, and a 400 is usually a filter or column problem.
func dataTableAPIError(err error, resolution config.Resolution, action string) error {
	if n8n.IsForbidden(err) {
		return fmt.Errorf("%s denied data table %s (403): the credential lacks the scope for it, or the instance is not licensed for data tables; run %s to inspect access",
			resolution.URL, action, discoverHint(dataTableResource))
	}
	if n8n.IsNotFound(err) {
		return fmt.Errorf("%s has no such data table, row or column (404): check the table ID with 'n8n data-table list' and the column ID with 'n8n data-table column list TABLE_ID'", resolution.URL)
	}
	if n8n.IsConflict(err) {
		return fmt.Errorf("%s refused data table %s (409): a table or column with that name already exists; list them first or pick another name", resolution.URL, action)
	}
	return apiError(err, resolution, dataTableResource)
}
