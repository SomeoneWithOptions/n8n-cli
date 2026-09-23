## n8n data-table column create

Add a column to a data table

### Synopsis

Add one column to an existing data table. The type is one of string, number, boolean, date
and cannot be changed afterwards; recreate the column to change it.

Without --index the column is appended after the existing ones. Existing rows
get an empty value in the new column. A name already used in the table comes
back as a 409. Requires dataTableColumn:create.

```
n8n data-table column create <table-id> <name> <type> [flags]
```

### Examples

```
  n8n data-table column create TABLE_ID status string
  n8n data-table column create TABLE_ID priority number --index 0
  n8n data-table column create TABLE_ID signed_at date --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for create
      --index int        zero-based position for the column (default: appended after the existing columns)
      --output string    output format: text or json (JSON is the created column) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n data-table column](n8n_data-table_column.md)	 - Manage the columns of a data table

