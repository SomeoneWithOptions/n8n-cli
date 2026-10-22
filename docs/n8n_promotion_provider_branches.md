## n8n promotion provider branches

List branches of a GitLab repository

### Synopsis

Discover GitLab repositories or branches using a saved provider with gitConnection:discover.
Start with 'n8n promotion provider list', then repositories, then branches.
Pass the raw fullPath (for example org/repo) or GitLab project ID to branches;
the CLI escapes it. Use remoteUrl from repositories as a connection target.
No saved connection is required and no content changes. Other provider types
do not support discovery. Text lists repository targets or branch names and
defaults; JSON returns data and nextCursor. Follow nextCursor even after a
short page. --all follows cursors up to 10,000 items.

With --all, JSON output adds a collection object. Its truncated field is true,
and a warning goes to stderr, when the walk stopped at the 10,000-item limit
without reaching the end. nextCursor then resumes the walk when the limit fell
between pages; it stays empty when the limit fell inside a page, because
resuming from the next page would skip items: narrow the filters or page
explicitly with --limit and --cursor instead.

```
n8n promotion provider branches <provider-id> <repository-path> [flags]
```

### Examples

```
  n8n promotion provider repositories PROVIDER_ID --search automation --all
  n8n promotion provider branches PROVIDER_ID org/repo --output json
```

### Options

```
      --all              follow all cursors, including short pages (default: one page; maximum 10,000 items)
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string    cursor from a previous page (default: first page)
  -h, --help             help for branches
      --limit int        items per API page, 1 to 250 (default: server default of 100; GitLab returns at most 100)
      --output string    output format: text or json (default: text; JSON includes data and nextCursor) (default "text")
      --search string    repository or branch search string, at most 255 characters (default: no filter)
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion provider](n8n_promotion_provider.md)	 - Manage promotion providers

