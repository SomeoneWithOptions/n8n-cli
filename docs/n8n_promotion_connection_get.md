## n8n promotion connection get

Show one promotion connection

### Synopsis

Show one promotion connection by ID: its name, scope, remote target,
provider, both direction configs, and timestamps. Secrets are never
returned. Find the ID with 'n8n promotion connection list'. Requires
gitConnection:read.

```
n8n promotion connection get <connection-id> [flags]
```

### Examples

```
  n8n promotion connection get CONNECTION_ID
  n8n promotion connection get CONNECTION_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the connection object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion connection](n8n_promotion_connection.md)	 - Manage promotion connections

