## n8n data-table get

Show one data table and its columns

### Synopsis

Show one data table: its name, owning project, storage size, and the columns
that define it, in column order.

The reported size is physical storage including indexes. It does not shrink the
moment rows are deleted and can be a few seconds stale, so do not read it as a
row count; use 'n8n data-table row list' for the rows themselves. Requires
dataTable:read.

```
n8n data-table get <table-id> [flags]
```

### Examples

```
  n8n data-table get TABLE_ID
  n8n data-table get TABLE_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the table object with its columns) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n data-table](n8n_data-table.md)	 - Manage data tables, their rows and their columns

