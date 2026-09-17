## n8n role update

Fully replace a custom role

### Synopsis

Fully replace a custom role's writable fields from strict JSON supplied by
--input. This is a PUT: displayName, description, and scopes are all required,
even when unchanged. Set description to null to remove it and scopes to [] to
remove every scope. roleType and slug cannot be changed.

System roles are immutable. Global roles require role:manage; project roles
require role:manageProject. Run 'n8n role get SLUG --output json' before building
the replacement document.

```
n8n role update <role-slug> [flags]
```

### Examples

```
  n8n role update global:custom --input role-update.json
  printf '%s\n' '{"displayName":"Auditor","description":null,"scopes":[]}' | n8n role update global:custom --input - --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for update
      --input string     role JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the role returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n role](n8n_role.md)	 - Manage global and project roles

