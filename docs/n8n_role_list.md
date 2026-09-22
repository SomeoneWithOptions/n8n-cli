## n8n role list

List global and project roles

### Synopsis

List every role visible to the selected credential, grouped into global and
project roles. Text and JSON output expose each role's licensed field so an
unlicensed role is not mistaken for an assignable one.

Pass --with-usage-count to include user and project assignment counts. Use a
returned slug with get, update, or delete. Requires role:list.

```
n8n role list [flags]
```

### Examples

```
  n8n role list
  n8n role list --with-usage-count
  n8n role list --with-usage-count --output json
```

### Options

```
      --context string     saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help               help for list
      --output string      output format: text or json (JSON has global and project role arrays) (default "text")
      --url string         instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --with-usage-count   include user and project assignment counts (default: counts are omitted by the API)
```

### SEE ALSO

* [n8n role](n8n_role.md)	 - Manage global and project roles

