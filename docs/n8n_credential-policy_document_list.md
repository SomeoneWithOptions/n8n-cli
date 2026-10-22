## n8n credential-policy document list

List credential type policy documents

### Synopsis

List reusable documents in cursor pages (default 100, maximum 250). Use --cursor for another page, or --all to collect up to 10,000 documents. Start here to find POLICY_ID for get, set, and delete. With --all, JSON output adds a collection object. Its truncated field is true,
and a warning goes to stderr, when the walk stopped at the 10,000-item limit
without reaching the end. nextCursor then resumes the walk when the limit fell
between pages; it stays empty when the limit fell inside a page, because
resuming from the next page would skip items: narrow the filters or page
explicitly with --limit and --cursor instead. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy document list [flags]
```

### Examples

```
  n8n credential-policy document list
```

### Options

```
      --all              follow cursor pages up to 10,000 documents (default: false)
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string    cursor returned by a previous list (default: first page)
  -h, --help             help for list
      --limit int        documents per page, 1 to 250 (default: 100) (default 100)
      --output string    output format: text or json (default: text; JSON contains the full response) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n credential-policy document](n8n_credential-policy_document.md)	 - Manage reusable credential type policy documents

