## n8n rate-limit get

Get Public API rate-limit settings

### Synopsis

Get the effective Public API rate-limit settings and the fields environment
variables manage. --output json is the starting point for 'set --input': it
round-trips, and set drops the read-only envManagedFields before sending.

Requires securitySettings:manage and n8n 2.47 or later.

```
n8n rate-limit get [flags]
```

### Examples

```
  n8n rate-limit get
  n8n rate-limit get --output json
  n8n rate-limit get --output json > rate-limits.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the complete settings document with envManagedFields) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n rate-limit](n8n_rate-limit.md)	 - Manage Public API rate limits

