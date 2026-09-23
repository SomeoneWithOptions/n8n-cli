## n8n promotion checkout disconnect

Remove one promotion checkout

### Synopsis

Remove the local checkout of one direction of a promotion connection.
The direction argument is apply or promote. The connection, its
configs, and its credentials are retained, and linked projects stay
linked, but promote and apply stop working for that direction until
'clone' runs again.

Interactive runs ask for confirmation; non-interactive runs require
--yes. Requires gitConnection:clone.

```
n8n promotion checkout disconnect <connection-id> <direction> [flags]
```

### Examples

```
  n8n promotion checkout disconnect CONNECTION_ID apply
  n8n promotion checkout disconnect CONNECTION_ID promote --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for disconnect
      --output string    output format: text or json (JSON is the checkout returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm checkout removal without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion checkout](n8n_promotion_checkout.md)	 - Manage promotion checkouts

