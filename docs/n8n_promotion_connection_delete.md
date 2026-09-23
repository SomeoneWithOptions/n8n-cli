## n8n promotion connection delete

Permanently delete a promotion connection

### Synopsis

Permanently delete one promotion connection. Its direction configs
and local checkouts can go with it. Linked projects stay on the
instance but lose their remote; promote and apply stop working until a
connection is created and cloned again. This cannot be undone.

To keep the connection and drop only a checkout, use 'n8n promotion
checkout disconnect' instead. Interactive runs ask for confirmation;
non-interactive runs require --yes. Requires gitConnection:delete.

```
n8n promotion connection delete <connection-id> [flags]
```

### Examples

```
  n8n promotion connection delete CONNECTION_ID
  n8n promotion connection delete CONNECTION_ID --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (JSON is the deletion acknowledgement) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm permanent connection and checkout deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion connection](n8n_promotion_connection.md)	 - Manage promotion connections

