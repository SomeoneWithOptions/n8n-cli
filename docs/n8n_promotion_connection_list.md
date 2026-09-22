## n8n promotion connection list

List promotion connections

### Synopsis

List the promotion connections visible to the selected credential,
one cursor-paginated page at a time. Filter with --scope (instance or
projects) and --provider-id; filters are preserved across --all.
Use --all to follow every cursor, capped at 10,000 connections.

Use a returned ID with get, update, delete, config, checkout, project,
promote, and apply. Requires gitConnection:list.

```
n8n promotion connection list [flags]
```

### Examples

```
  n8n promotion connection list
  n8n promotion connection list --scope projects --provider-id PROVIDER_ID
  n8n promotion connection list --limit 50 --output json
  n8n promotion connection list --all
```

### Options

```
      --all                  follow every page instead of one (maximum 10,000 connections)
      --context string       saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string        pagination cursor returned by a previous list (default: the first page)
  -h, --help                 help for list
      --limit int            connections per API page, 1 to 250 (default: server default of 100)
      --output string        output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --provider-id string   filter by provider ID from 'n8n promotion provider list' (default: all providers)
      --scope string         filter by connection scope: instance or projects (default: all scopes)
      --url string           instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion connection](n8n_promotion_connection.md)	 - Manage promotion connections

