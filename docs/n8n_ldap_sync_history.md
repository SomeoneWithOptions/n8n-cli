## n8n ldap sync history

List LDAP synchronization history

### Synopsis

List LDAP synchronization records newest first, one cursor-paginated page at
a time. Pass nextCursor to --cursor, or use --all to follow pages up to the
10,000-record safety cap. Each record reports mode, status, timing, scanned users,
and create, update, and disable counts. Requires ldap:sync and the LDAP license.

```
n8n ldap sync history [flags]
```

### Examples

```
  n8n ldap sync history
  n8n ldap sync history --limit 25 --cursor NEXT_CURSOR
  n8n ldap sync history --all --output json
```

### Options

```
      --all              follow every history page instead of one (maximum 10,000 records)
      --context string   saved context to use (default: the current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by previous history request (default: first page)
  -h, --help             help for history
      --limit int        records per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n ldap sync](n8n_ldap_sync.md)	 - Inspect or run LDAP synchronization

