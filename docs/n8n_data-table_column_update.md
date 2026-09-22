## n8n data-table column update

Rename or reorder a column

### Synopsis

Rename one column, move it to another position, or both. At least one of --name
and --index is required; the type of a column cannot be changed.

Renaming a column changes the key the row commands use for it, so update any
saved --set, --where and --input documents afterwards, and check workflows that
read the table. --index is zero-based and shifts the other columns around it.
Find column IDs with 'n8n data-table column list'. Requires
dataTableColumn:update.

```
n8n data-table column update <table-id> <column-id> [flags]
```

### Examples

```
  n8n data-table column update TABLE_ID COLUMN_ID --name state
  n8n data-table column update TABLE_ID COLUMN_ID --index 0
  n8n data-table column update TABLE_ID COLUMN_ID --name state --index 1 --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for update
      --index int        new zero-based position (required unless --name is given)
      --name string      new column name (required unless --index is given)
      --output string    output format: text or json (JSON is the updated column) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n data-table column](n8n_data-table_column.md)	 - Manage the columns of a data table

