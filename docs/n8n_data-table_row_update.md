## n8n data-table row update

Update every row matching a filter

### Synopsis

Write column values into every row matching the filter. Select the rows with
repeatable --where expressions or with --filter, and give the new values with
repeatable --set COLUMN=VALUE or a JSON object through --input. A selection is
required: this command will not rewrite a whole table by accident.

Run it with --dry-run first on anything important. A dry run asks the server
for the rows it would touch and returns each one twice, marked dryRunState
before and after, without writing anything. Rows that do not exist are not
created; use 'n8n data-table row upsert' for that. Requires
dataTableRow:update.

```
n8n data-table row update <table-id> [flags]
```

### Examples

```
  n8n data-table row update TABLE_ID --where status=pending --set status=done --dry-run
  n8n data-table row update TABLE_ID --where status=pending --set status=done
  n8n data-table row update TABLE_ID --where age=gte=65 --set discount=true --return-data
  n8n data-table row update TABLE_ID --filter '{"filters":[{"columnName":"id","condition":"eq","value":1}]}' --input values.json
```

### Options

```
      --context string      saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --dry-run             ask the server to report the change without persisting it (implies --return-data)
      --filter string       whole filter object as JSON, e.g. '{"type":"and","filters":[{"columnName":"status","condition":"eq","value":"active"}]}' (cannot combine with --where)
  -h, --help                help for update
      --input string        column values as a JSON object, from a file path or - for stdin (cannot combine with --set; maximum 1 MiB)
      --match string        how several --where expressions combine: and or or (default "and")
      --output string       output format: text or json (JSON contains the acknowledgement and any returned rows) (default "text")
      --return-data         ask the API for the affected rows instead of a bare acknowledgement
      --set stringArray     column value to write as COLUMN=VALUE, repeatable (cannot combine with --input)
      --url string          instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --where stringArray   row condition as COLUMN=VALUE or COLUMN=CONDITION=VALUE, repeatable, conditions: eq, neq, like, ilike, gt, gte, lt, lte (required unless --filter is given)
```

### SEE ALSO

* [n8n data-table row](n8n_data-table_row.md)	 - Read and change the rows of a data table

