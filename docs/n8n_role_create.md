## n8n role create

Create a custom role

### Synopsis

Create one custom role from strict JSON supplied by --input. Required fields are
displayName (2-100 characters), roleType (global or project), and scopes (an
array that may be empty). description is optional and limited to 500 characters.

Unknown fields and trailing JSON documents are refused before transport. Global
roles require role:manage; project roles require role:manageProject.

```
n8n role create [flags]
```

### Examples

```
  n8n role create --input role.json
  printf '%s\n' '{"displayName":"Auditor","roleType":"global","scopes":["workflow:read"]}' | n8n role create --input - --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for create
      --input string     role JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the role returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n role](n8n_role.md)	 - Manage global and project roles

