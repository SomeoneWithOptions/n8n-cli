## n8n promotion connection update

Update a promotion connection

### Synopsis

Update one promotion connection from strict JSON supplied by --input.
Only the fields present are changed; at least one of name, target, or
providerId is required. Direction configs are not patched here: use
'n8n promotion config apply set' and 'n8n promotion config promote
set' for those. Requires gitConnection:update.

```
n8n promotion connection update <connection-id> [flags]
```

### Examples

```
  n8n promotion connection update CONNECTION_ID --input connection-update.json
  printf '%s\n' '{"name":"staging"}' | n8n promotion connection update CONNECTION_ID --input -
  n8n promotion connection update CONNECTION_ID --input connection-update.json --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for update
      --input string     promotion connection JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the connection returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n promotion connection](n8n_promotion_connection.md)	 - Manage promotion connections

