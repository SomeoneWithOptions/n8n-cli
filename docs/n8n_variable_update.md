## n8n variable update

Fully replace a variable

### Synopsis

Fully replace one variable by ID. This is a PUT, so --key and --value are both
required even when one is unchanged. Omitting --project-id replaces the current
scope with global scope; pass its project ID to keep or select project scope.

A key already used in the resulting scope fails with a conflict. An explicitly
empty value is valid. Submitted values never appear in diagnostics or success
acknowledgements. Requires variable:update.

```
n8n variable update <variable-id> [flags]
```

### Examples

```
  n8n variable update VARIABLE_ID --key API_HOST --value https://api.internal
  n8n variable update VARIABLE_ID --key API_HOST --value "" --project-id PROJECT_ID
  n8n variable update VARIABLE_ID --key API_HOST --value VALUE --output json
```

### Options

```
      --context string      saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help                help for update
      --key string          complete replacement key (required, even when unchanged)
      --output string       output format: text or json (acknowledgement excludes the submitted value) (default "text")
      --project-id string   project scope ID (default: global scope; on update this replaces current scope)
      --url string          instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --value string        complete variable value; pass an empty string to store an empty value (required, not repeated in diagnostics)
```

### SEE ALSO

* [n8n variable](n8n_variable.md)	 - Manage instance and project variables

