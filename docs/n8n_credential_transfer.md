## n8n credential transfer

Transfer a credential to another project

### Synopsis

Transfer an owned credential to another project. This changes who can access it and
may break workflows in the source project. Interactive runs ask for confirmation;
non-interactive runs require --yes. Requires credential:move and suitable project
permissions.

```
n8n credential transfer <credential-id> [flags]
```

### Examples

```
  n8n credential transfer CREDENTIAL_ID --destination-project PROJECT_ID
  n8n credential transfer CREDENTIAL_ID --destination-project PROJECT_ID --yes --output json
```

### Options

```
      --context string               saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --destination-project string   destination project ID (required)
  -h, --help                         help for transfer
      --output string                output format: text or json (default "text")
      --url string                   instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes                          confirm transfer without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n credential](n8n_credential.md)	 - Manage node credentials without exposing secrets

