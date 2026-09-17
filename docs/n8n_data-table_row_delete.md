## n8n data-table row delete

Permanently delete the rows matching a filter

### Synopsis

Permanently delete every row matching the filter. Select the rows with
repeatable --where expressions or with --filter; the API itself requires a
filter, which is what keeps this from emptying a table by accident. Use
'n8n data-table row clear' when emptying it is the intent.

--dry-run asks the server which rows would go and deletes nothing, so it needs
no confirmation; a real delete asks for one, or takes --yes. Deleted rows
cannot be recovered. Requires dataTableRow:delete.

```
n8n data-table row delete <table-id> [flags]
```

### Examples

```
  n8n data-table row delete TABLE_ID --where status=archived --dry-run
  n8n data-table row delete TABLE_ID --where status=archived --yes
  n8n data-table row delete TABLE_ID --where age=lt=18 --match and --where status=inactive --yes
  n8n data-table row delete TABLE_ID --where id=1 --return-data --yes --output json
```

### Options

```
      --context string      saved context to use (default: the current context; see 'n8n config context list')
      --dry-run             report which rows would be deleted without deleting them (implies --return-data; no confirmation needed)
      --filter string       whole filter object as JSON, e.g. '{"type":"and","filters":[{"columnName":"status","condition":"eq","value":"active"}]}' (cannot combine with --where)
  -h, --help                help for delete
      --match string        how several --where expressions combine: and or or (default "and")
      --output string       output format: text or json (JSON contains the acknowledgement and any returned rows) (default "text")
      --return-data         ask the API for the deleted rows instead of a bare acknowledgement
      --url string          instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --where stringArray   row condition as COLUMN=VALUE or COLUMN=CONDITION=VALUE, repeatable, conditions: eq, neq, like, ilike, gt, gte, lt, lte (required unless --filter is given)
      --yes                 confirm permanent deletion of the matching rows without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n data-table row](n8n_data-table_row.md)	 - Read and change the rows of a data table

