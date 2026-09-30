## n8n credential list

List credential metadata

### Synopsis

List credential metadata from the selected instance. Stored credential data is
never returned or printed. n8n restricts this endpoint to instance owners and
admins. Use --all to follow cursors, capped at 10,000 credentials.

Requires credential:list. A 403 can mean either missing scope or insufficient
instance role; inspect capabilities with 'n8n discover --resource credential'.

Script views print rows only, with no header, count or blank lines, so stdout
pipes cleanly; the next cursor and any empty-result hint go to stderr, and an
empty result prints nothing:
  --quiet  one credential ID per line, printed exactly
  --brief  ID, name and type, separated by a literal tab
--brief escapes backslash, tab, newline, carriage return and other control
characters in fields as \\, \t, \n, \r and \uXXXX; use --output json for exact
values. The views combine neither with each other nor with --output json.

With --all, JSON output adds a collection object. Its truncated field is true,
and a warning goes to stderr, when the walk stopped at the 10,000-item limit
without reaching the end. nextCursor then resumes the walk when the limit fell
between pages; it stays empty when the limit fell inside a page, because
resuming from the next page would skip items: narrow the filters or page
explicitly with --limit and --cursor instead.

```
n8n credential list [flags]
```

### Examples

```
  n8n credential list
  n8n credential list --brief
  n8n credential list --all --quiet
  n8n credential list --limit 50 --output json
  n8n credential list --all
```

### Options

```
      --all              follow all pages (maximum 10,000 credentials; hitting it warns and sets JSON collection.truncated)
      --brief            print only ID, name and type, tab-separated and escaped, with no header (default: full table)
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous list
  -h, --help             help for list
      --limit int        credentials per API page (default: server default)
      --output string    output format: text or json (JSON is a page object with data and nextCursor, plus collection with --all) (default "text")
      --quiet            print only credential IDs, one per line, with no header (default: full table)
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n credential](n8n_credential.md)	 - Manage node credentials without exposing secrets

