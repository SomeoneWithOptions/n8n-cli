## n8n credential test

Test stored credential data

### Synopsis

Ask n8n to test one credential using its stored data. No credential secret is sent
by the CLI or returned in output. Requires credential:read and access to the
credential. A test result with status Error is still a successful API call.

```
n8n credential test <credential-id> [flags]
```

### Examples

```
  n8n credential test CREDENTIAL_ID
  n8n credential test CREDENTIAL_ID --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for test
      --output string    output format: text or json (output never contains credential data) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n credential](n8n_credential.md)	 - Manage node credentials without exposing secrets

