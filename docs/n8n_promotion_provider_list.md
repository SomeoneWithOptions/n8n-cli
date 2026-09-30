## n8n promotion provider list

List promotion providers

### Synopsis

List the promotion providers visible to the selected credential, one
cursor-paginated page at a time. Use --all to follow every cursor,
capped at 10,000 providers.

Use a returned ID with get, update, delete, and 'connection create'.
Requires gitConnection:list.

With --all, JSON output adds a collection object. Its truncated field is true,
and a warning goes to stderr, when the walk stopped at the 10,000-item limit
without reaching the end. nextCursor then resumes the walk when the limit fell
between pages; it stays empty when the limit fell inside a page, because
resuming from the next page would skip items: narrow the filters or page
explicitly with --limit and --cursor instead.

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
      --all              follow every page instead of one (maximum 10,000 providers; hitting it warns and sets JSON collection.truncated)
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous list (default: the first page)
  -h, --help             help for list
      --limit int        providers per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON is a page object with data and nextCursor, plus collection with --all) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion provider](n8n_promotion_provider.md)	 - Manage promotion providers

