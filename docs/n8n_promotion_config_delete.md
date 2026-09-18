## n8n promotion config delete

Delete one direction config

### Synopsis

Delete the apply or promote direction config of one promotion
connection. The direction argument is apply or promote. Removing a
config can remove its local checkout.

Interactive runs ask for confirmation; non-interactive runs require
--yes. Requires gitConnection:update.

```
n8n promotion config delete <connection-id> <direction> [flags]
```

### Examples

```
  n8n promotion config delete CONNECTION_ID apply
  n8n promotion config delete CONNECTION_ID promote --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (JSON is the deletion acknowledgement) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm config deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion config](n8n_promotion_config.md)	 - Manage promotion direction configs

