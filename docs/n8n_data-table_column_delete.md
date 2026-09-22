## n8n data-table column delete

Permanently delete a column and its data

### Synopsis

Permanently delete one column from a data table. The value that column holds is
deleted in every row, not only in the schema, and it cannot be recovered. The
remaining columns close the gap in the index order.

Workflows that read or write this column start failing, so check them first.
Copy the data with 'n8n data-table row list --all --output json' if it matters.
Interactive runs ask for confirmation. Non-interactive runs require --yes.
Requires dataTableColumn:delete.

```
n8n data-table column delete <table-id> <column-id> [flags]
```

### Examples

```
  n8n data-table column delete TABLE_ID COLUMN_ID
  n8n data-table column delete TABLE_ID COLUMN_ID --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (acknowledgement contains the table and column) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm permanent deletion of the column and its data without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n data-table column](n8n_data-table_column.md)	 - Manage the columns of a data table

