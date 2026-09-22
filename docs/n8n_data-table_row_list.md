## n8n data-table row list

List the rows of a data table

### Synopsis

List rows from one data table, one cursor-paginated page at a time. Pass a
returned cursor to --cursor, or use --all to follow every cursor, capped at
10,000 rows.

Narrow the rows with repeatable --where expressions (combined with --match and
or or), or with the whole filter object as JSON through --filter. --search
matches text across every string column, and --sort-by takes columnName:asc or
columnName:desc. Text output prints the system columns first, then the
user-defined ones; --output json is the faithful copy of the row data and the
right input for a backup. Requires dataTableRow:read.

```
n8n data-table row list <table-id> [flags]
```

### Examples

```
  n8n data-table row list TABLE_ID
  n8n data-table row list TABLE_ID --where status=active --limit 50
  n8n data-table row list TABLE_ID --where age=gte=30 --sort-by age:desc
  n8n data-table row list TABLE_ID --search example.com
  n8n data-table row list TABLE_ID --all --output json
```

### Options

```
      --all                 follow every page instead of one (maximum 10,000 rows)
      --context string      saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string       pagination cursor returned by a previous list (default: the first page)
      --filter string       whole filter object as JSON, e.g. '{"type":"and","filters":[{"columnName":"status","condition":"eq","value":"active"}]}' (cannot combine with --where)
  -h, --help                help for list
      --limit int           rows per API page, 1 to 250 (default: server default of 100)
      --match string        how several --where expressions combine: and or or (default "and")
      --output string       output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --search string       match this text across every string column (default: no search)
      --sort-by string      sort order as columnName:asc or columnName:desc (default: the server's order)
      --url string          instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --where stringArray   row condition as COLUMN=VALUE or COLUMN=CONDITION=VALUE, repeatable, conditions: eq, neq, like, ilike, gt, gte, lt, lte
```

### SEE ALSO

* [n8n data-table row](n8n_data-table_row.md)	 - Read and change the rows of a data table

