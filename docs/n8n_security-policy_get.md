## n8n security-policy get

Get effective security policy

### Synopsis

Get the effective instance security policy and the read-only counts of published
and shared personal resources affected by it. Use --output json as the starting
point for a read-edit-set workflow; 'set' accepts the returned usage fields but
the server ignores them on write.

Requires securitySettings:manage and the licensed Personal Space Policy feature.
A policy managed by environment variables remains readable.

```
n8n security-policy get [flags]
```

### Examples

```
  n8n security-policy get
  n8n security-policy get --output json
  n8n security-policy get --output json > security-policy.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the complete policy plus read-only usage counts) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n security-policy](n8n_security-policy.md)	 - Manage instance security policy

