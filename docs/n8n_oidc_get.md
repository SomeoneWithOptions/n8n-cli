## n8n oidc get

Get OIDC SSO configuration

### Synopsis

Get every OIDC SSO setting exposed by n8n. The client secret is never
returned: clientSecret is empty when unset or n8n's redacted placeholder when
configured. Keep that placeholder unchanged in a GET-edit-PUT workflow to
preserve the stored secret. Text output only reports secret status.

Requires oidc:manage and the OIDC license.

```
n8n oidc get [flags]
```

### Examples

```
  n8n oidc get
  n8n oidc get --output json
  umask 077 && n8n oidc get --output json > oidc.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON contains only a client-secret placeholder, never the stored secret) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n oidc](n8n_oidc.md)	 - Manage licensed OIDC SSO settings

