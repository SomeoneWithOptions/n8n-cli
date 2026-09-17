## n8n data-table list

List data tables

### Synopsis

List the data tables the credential can see, one cursor-paginated page at a
time. Pass a returned cursor to --cursor for the next page, or use --all to
follow every cursor, capped at 10,000 tables.

Use this to find the table ID every other command takes. --name is the only
filter the API documents. --sort-by takes field:asc or field:desc, for example
name:asc or size:desc, where size orders by the reported sizeBytes. Requires
dataTable:list and API-key authentication.

```
n8n data-table list [flags]
```

### Examples

```
  n8n data-table list
  n8n data-table list --name customers
  n8n data-table list --sort-by size:desc --limit 50 --output json
  n8n data-table list --all
```

### Options

```
      --all              follow every page instead of one (maximum 10,000 tables)
      --context string   saved context to use (default: the current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous list (default: the first page)
  -h, --help             help for list
      --limit int        tables per API page, 1 to 250 (default: server default of 100)
      --name string      only tables with this name (default: every table)
      --output string    output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --sort-by string   sort order as field:asc or field:desc, e.g. name:asc or size:desc (default: the server's order)
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n data-table](n8n_data-table.md)	 - Manage data tables, their rows and their columns

