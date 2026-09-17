## n8n variable create

Create a variable

### Synopsis

Create one variable with a complete key and value. Omit --project-id for a
global variable, or pass a project ID for project scope. A key must be unique
within that scope; a duplicate fails with a conflict.

--value is required, though an explicitly empty value is valid. The submitted
value is sent to n8n but never repeated in CLI diagnostics or acknowledgement
output. Requires variable:create.

```
n8n variable create <key> [flags]
```

### Examples

```
  n8n variable create API_HOST --value https://api.example.com
  n8n variable create OPTIONAL_VALUE --value ""
  n8n variable create API_HOST --value https://api.example.com --project-id PROJECT_ID --output json
```

### Options

```
      --context string      saved context to use (default: the current context; see 'n8n config context list')
  -h, --help                help for create
      --output string       output format: text or json (acknowledgement excludes the submitted value) (default "text")
      --project-id string   project scope ID (default: global scope; on update this replaces current scope)
      --url string          instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --value string        complete variable value; pass an empty string to store an empty value (required, not repeated in diagnostics)
```

### SEE ALSO

* [n8n variable](n8n_variable.md)	 - Manage instance and project variables

