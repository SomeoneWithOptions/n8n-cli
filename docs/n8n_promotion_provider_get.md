## n8n promotion provider get

Show one promotion provider

### Synopsis

Show one promotion provider by ID. The response is the public provider
document: name, type, auth type, configuration, and timestamps.
Secrets are never returned. Find the ID with 'n8n promotion provider
list'. Requires gitConnection:read.

```
n8n promotion provider get <provider-id> [flags]
```

### Examples

```
  n8n promotion provider get PROVIDER_ID
  n8n promotion provider get PROVIDER_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the provider object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion provider](n8n_promotion_provider.md)	 - Manage promotion providers

