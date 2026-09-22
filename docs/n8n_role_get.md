## n8n role get

Get one role

### Synopsis

Get one global or project role by its exact slug. Output includes scopes, whether
the role is licensed, and whether it is an immutable system role. Pass
--with-usage-count to include user and project assignment counts.

Use 'n8n role list' to discover slugs. This call is read-only and requires
role:read.

```
n8n role get <role-slug> [flags]
```

### Examples

```
  n8n role get global:member
  n8n role get project:admin --with-usage-count
  n8n role get global:member --output json
```

### Options

```
      --context string     saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help               help for get
      --output string      output format: text or json (JSON is the complete role object) (default "text")
      --url string         instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --with-usage-count   include user and project assignment counts (default: counts are omitted by the API)
```

### SEE ALSO

* [n8n role](n8n_role.md)	 - Manage global and project roles

