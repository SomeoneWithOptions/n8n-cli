## n8n data-table row upsert

Update the row matching a filter, or insert it

### Synopsis

Update the row matching the filter, or insert a new row when nothing matches.
Select with repeatable --where expressions or --filter, and give the values
with repeatable --set COLUMN=VALUE or a JSON object through --input.

The inserted row is built from the values given, not from the filter, so
include the matched column in --set when the new row should carry it. Use
--dry-run to see what would happen first. Requires dataTableRow:upsert.

```
n8n data-table row upsert <table-id> [flags]
```

### Examples

```
  n8n data-table row upsert TABLE_ID --where email=a@example.com --set email=a@example.com --set name=Jane
  n8n data-table row upsert TABLE_ID --where email=a@example.com --set name=Jane --return-data
  n8n data-table row upsert TABLE_ID --where email=a@example.com --set name=Jane --dry-run --output json
```

### Options

```
      --context string      saved context to use (default: the current context; see 'n8n config context list')
      --dry-run             ask the server to report the change without persisting it (implies --return-data)
      --filter string       whole filter object as JSON, e.g. '{"type":"and","filters":[{"columnName":"status","condition":"eq","value":"active"}]}' (cannot combine with --where)
  -h, --help                help for upsert
      --input string        column values as a JSON object, from a file path or - for stdin (cannot combine with --set; maximum 1 MiB)
      --match string        how several --where expressions combine: and or or (default "and")
      --output string       output format: text or json (JSON contains the acknowledgement and any returned rows) (default "text")
      --return-data         ask the API for the affected rows instead of a bare acknowledgement
      --set stringArray     column value to write as COLUMN=VALUE, repeatable (cannot combine with --input)
      --url string          instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --where stringArray   row condition as COLUMN=VALUE or COLUMN=CONDITION=VALUE, repeatable, conditions: eq, neq, like, ilike, gt, gte, lt, lte (required unless --filter is given)
```

### SEE ALSO

* [n8n data-table row](n8n_data-table_row.md)	 - Read and change the rows of a data table

