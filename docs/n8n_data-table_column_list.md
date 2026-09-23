## n8n data-table column list

List the columns of a data table

### Synopsis

List every user-defined column of one data table with its ID, type and
position. The response is a plain array, not a page, so there is no pagination
here.

Use the column IDs with 'n8n data-table column update' and
'n8n data-table column delete', and the column names with the row commands.
Requires dataTableColumn:read.

```
n8n data-table column list <table-id> [flags]
```

### Examples

```
  n8n data-table column list TABLE_ID
  n8n data-table column list TABLE_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for list
      --output string    output format: text or json (JSON is the column array) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n data-table column](n8n_data-table_column.md)	 - Manage the columns of a data table

