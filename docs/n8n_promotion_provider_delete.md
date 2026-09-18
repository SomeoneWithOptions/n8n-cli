## n8n promotion provider delete

Permanently delete a promotion provider

### Synopsis

Permanently delete one promotion provider. Attached connections keep
their stored content but lose their credential, so promote, apply, and
checkout commands stop working until a provider is attached again.
This cannot be undone.

Interactive runs ask for confirmation; non-interactive runs require
--yes. Requires gitConnection:delete.

```
n8n promotion provider delete <provider-id> [flags]
```

### Examples

```
  n8n promotion provider delete PROVIDER_ID
  n8n promotion provider delete PROVIDER_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (JSON is the deletion acknowledgement) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm permanent provider deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion provider](n8n_promotion_provider.md)	 - Manage promotion providers

