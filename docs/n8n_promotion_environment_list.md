## n8n promotion environment list

List promotion environments

### Synopsis

List the promotion environments visible to the selected credential,
one cursor-paginated page at a time. Filter with --scope (instance or
projects), --direction (apply or promote), and --provider-id; filters
are preserved across --all. Use --all to follow every cursor, capped at
10,000 environments.

Use a returned ID with get, update, delete, connect, disconnect, and
project. Requires gitConnection:list.

With --all, JSON output adds a collection object. Its truncated field is true,
and a warning goes to stderr, when the walk stopped at the 10,000-item limit
without reaching the end. nextCursor then resumes the walk when the limit fell
between pages; it stays empty when the limit fell inside a page, because
resuming from the next page would skip items: narrow the filters or page
explicitly with --limit and --cursor instead.

```
n8n promotion environment list [flags]
```

### Examples

```
  n8n promotion environment list
  n8n promotion environment list --scope instance --direction apply
  n8n promotion environment list --provider-id PROVIDER_ID --output json
  n8n promotion environment list --all
```

### Options

```
      --all                  follow every page instead of one (maximum 10,000 environments; hitting it warns and sets JSON collection.truncated)
      --context string       saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string        pagination cursor returned by a previous list (default: the first page)
      --direction string     filter by direction: apply or promote (default: both directions)
  -h, --help                 help for list
      --limit int            environments per API page, 1 to 250 (default: server default of 100)
      --output string        output format: text or json (JSON is a page object with data and nextCursor, plus collection with --all) (default "text")
      --provider-id string   filter by provider ID from 'n8n promotion provider list' (default: all providers)
      --scope string         filter by environment scope: instance or projects (default: all scopes)
      --url string           instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)

