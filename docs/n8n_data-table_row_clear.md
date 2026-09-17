## n8n data-table row clear

Permanently delete every row of a data table

### Synopsis

Permanently delete every row of one data table. The table, its columns and its
ID stay, so workflows that use it keep working against an empty table. There is
no filter and no undo; the response reports how many rows were deleted.

Copy the rows first with 'n8n data-table row list --all --output json' if they
matter, or delete a subset with 'n8n data-table row delete --where ...', which
supports a dry run. Interactive runs ask for confirmation. Non-interactive runs
require --yes. Requires dataTableRow:delete.

```
n8n data-table row clear <table-id> [flags]
```

### Examples

```
  n8n data-table row clear TABLE_ID
  n8n data-table row clear TABLE_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for clear
      --output string    output format: text or json (JSON contains the deleted row count) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm deletion of every row without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n data-table row](n8n_data-table_row.md)	 - Read and change the rows of a data table

