## n8n credential get

Get credential metadata

### Synopsis

Get one credential by ID. n8n omits stored credential data, and the CLI drops
any unexpected data field before output. Requires credential:read and access to
the credential.

```
n8n credential get <credential-id> [flags]
```

### Examples

```
  n8n credential get CREDENTIAL_ID
  n8n credential get CREDENTIAL_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (output never contains credential data) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n credential](n8n_credential.md)	 - Manage node credentials without exposing secrets

