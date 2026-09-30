## n8n git-connection list

List Git connections

### Synopsis

List the Git connections visible to the selected credential, one
cursor-paginated page at a time. An instance holds at most one
connection, so this usually carries zero or one row. Use --all to
follow every cursor, capped at 10,000 connections.

Use a returned ID with get, update, delete, clone, disconnect, project,
push, and pull. Requires gitConnection:list.

With --all, JSON output adds a collection object. Its truncated field is true,
and a warning goes to stderr, when the walk stopped at the 10,000-item limit
without reaching the end. nextCursor then resumes the walk when the limit fell
between pages; it stays empty when the limit fell inside a page, because
resuming from the next page would skip items: narrow the filters or page
explicitly with --limit and --cursor instead.

```
n8n git-connection list [flags]
```

### Examples

```
  n8n git-connection list
  n8n git-connection list --limit 50 --output json
  n8n git-connection list --all
```

### Options

```
      --all              follow every page instead of one (maximum 10,000 connections; hitting it warns and sets JSON collection.truncated)
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous list (default: the first page)
  -h, --help             help for list
      --limit int        connections per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON is a page object with data and nextCursor, plus collection with --all) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n git-connection](n8n_git-connection.md)	 - Manage the Git connection, its projects, and push/pull sync

