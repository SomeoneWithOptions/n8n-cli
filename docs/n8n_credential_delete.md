## n8n credential delete

Permanently delete a credential

### Synopsis

Permanently delete an owned credential. Workflows that use it may stop working;
the operation does not delete those workflows. Interactive runs ask for
confirmation. Non-interactive runs require --yes. Requires credential:delete.

```
n8n credential delete <credential-id> [flags]
```

### Examples

```
  n8n credential delete CREDENTIAL_ID
  n8n credential delete CREDENTIAL_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (output never contains credential data) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm permanent deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n credential](n8n_credential.md)	 - Manage node credentials without exposing secrets

