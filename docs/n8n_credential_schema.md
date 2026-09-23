## n8n credential schema

Show credential data JSON Schema

### Synopsis

Show the JSON Schema for a credential type. Use it to construct the data object
for create or update without putting secret values in argv. Schema output is always
pretty-printed JSON. This endpoint declares no additional scope.

```
n8n credential schema <credential-type> [flags]
```

### Examples

```
  n8n credential schema githubApi
  n8n credential schema slackOAuth2Api --context production
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for schema
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n credential](n8n_credential.md)	 - Manage node credentials without exposing secrets

