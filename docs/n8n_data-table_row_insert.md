## n8n data-table row insert

Insert rows into a data table

### Synopsis

Insert one or more rows. Pass one row with repeatable --set COLUMN=VALUE, or a
JSON array of row objects (or one object) with --input FILE or --input -. The
two ways are mutually exclusive and one is required.

Values are typed by their JSON form: age=30 is a number, active=true a boolean,
note=null a null, and anything that is not valid JSON is sent as text. Columns
must already exist; add them with 'n8n data-table column create'. --return
selects what comes back: count, the inserted ids, or the full rows. The whole
batch is one request, so the API accepts or rejects it as a whole. Requires
dataTableRow:create.

```
n8n data-table row insert <table-id> [flags]
```

### Examples

```
  n8n data-table row insert TABLE_ID --set email=a@example.com --set age=30
  n8n data-table row insert TABLE_ID --set email=b@example.com --return all --output json
  n8n data-table row insert TABLE_ID --input rows.json --return id
  printf '%s\n' '[{"email":"c@example.com"}]' | n8n data-table row insert TABLE_ID --input -
```

### Options

```
      --context string    saved context to use (default: the current context; see 'n8n config context list')
  -h, --help              help for insert
      --input string      row JSON array or object, from a file path or - for stdin (cannot combine with --set; maximum 1 MiB)
      --output string     output format: text or json (JSON preserves the count, ids and rows the API sent) (default "text")
      --return string     what the API returns: count, id or all (default: the server default of count)
      --set stringArray   column value as COLUMN=VALUE for one row, repeatable (cannot combine with --input)
      --url string        instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n data-table row](n8n_data-table_row.md)	 - Read and change the rows of a data table

