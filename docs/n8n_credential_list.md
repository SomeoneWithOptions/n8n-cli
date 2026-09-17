## n8n credential list

List credential metadata

### Synopsis

List credential metadata from the selected instance. Stored credential data is
never returned or printed. n8n restricts this endpoint to instance owners and
admins. Use --all to follow cursors, capped at 10,000 credentials.

Requires credential:list. A 403 can mean either missing scope or insufficient
instance role; inspect capabilities with 'n8n discover --resource credential'.

```
n8n credential list [flags]
```

### Examples

```
  n8n credential list
  n8n credential list --limit 50 --output json
  n8n credential list --all
```

### Options

```
      --all              follow all pages (maximum 10,000 credentials)
      --context string   saved context to use (default: the current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous list
  -h, --help             help for list
      --limit int        credentials per API page (default: server default)
      --output string    output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n credential](n8n_credential.md)	 - Manage node credentials without exposing secrets

