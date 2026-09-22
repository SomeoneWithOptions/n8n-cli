## n8n data-table update

Rename a data table

### Synopsis

Rename one data table. The name is the only writable property of a table: its
columns are changed with 'n8n data-table column' and its contents with
'n8n data-table row'.

A name already used by another table in the same project comes back as a 409.
Names are limited to 128 characters. Requires dataTable:update.

```
n8n data-table update <table-id> [flags]
```

### Examples

```
  n8n data-table update TABLE_ID --name customers-2026
  n8n data-table update TABLE_ID --name archive --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for update
      --name string      new table name, up to 128 characters (required)
      --output string    output format: text or json (JSON is the updated table) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n data-table](n8n_data-table.md)	 - Manage data tables, their rows and their columns

