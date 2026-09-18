## n8n promotion provider list

List promotion providers

### Synopsis

List the promotion providers visible to the selected credential, one
cursor-paginated page at a time. Use --all to follow every cursor,
capped at 10,000 providers.

Use a returned ID with get, update, delete, and 'connection create'.
Requires gitConnection:list.

```
n8n promotion provider list [flags]
```

### Examples

```
  n8n promotion provider list
  n8n promotion provider list --limit 50 --output json
  n8n promotion provider list --all
```

### Options

```
      --all              follow every page instead of one (maximum 10,000 providers)
      --context string   saved context to use (default: the current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous list (default: the first page)
  -h, --help             help for list
      --limit int        providers per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n promotion provider](n8n_promotion_provider.md)	 - Manage promotion providers

