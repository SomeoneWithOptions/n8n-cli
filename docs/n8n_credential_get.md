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
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (output never contains credential data) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n credential](n8n_credential.md)	 - Manage node credentials without exposing secrets

