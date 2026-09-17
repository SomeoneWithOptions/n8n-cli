## n8n data-table delete

Permanently delete a data table and every row in it

### Synopsis

Permanently delete one data table. Every row it holds and every column it
defines go with it, and workflow nodes that read or write the table start
failing. This cannot be undone and the API offers no export step, so copy
anything worth keeping first with 'n8n data-table row list --all --output json'.

To empty a table but keep its structure, use 'n8n data-table row clear' instead.
Interactive runs ask for confirmation. Non-interactive runs require --yes.
Requires dataTable:delete.

```
n8n data-table delete <table-id> [flags]
```

### Examples

```
  n8n data-table delete TABLE_ID
  n8n data-table delete TABLE_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (acknowledgement contains the deleted table ID) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm permanent deletion of the table and its rows without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n data-table](n8n_data-table.md)	 - Manage data tables, their rows and their columns

