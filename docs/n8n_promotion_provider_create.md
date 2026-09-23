## n8n promotion provider create

Create a promotion provider

### Synopsis

Create one promotion provider from strict JSON supplied by --input.
Required fields are name (at most 128 characters), type (git), and an
auth object: {"authType":"ssh-key"} with an optional keyType
(ed25519 or rsa, default ed25519), or {"authType":"token",
"username":"...", "password":"..."}.

Credentials travel only as --input because secrets are never flags:
process lists and shell history can expose argv. Unknown fields and
trailing JSON documents are refused before transport. Requires
gitConnection:create. Follow with 'n8n promotion connection create'.

```
n8n promotion provider create [flags]
```

### Examples

```
  n8n promotion provider create --input provider.json
  printf '%s\n' '{"name":"github","type":"git","auth":{"authType":"ssh-key"}}' | n8n promotion provider create --input -
  n8n promotion provider create --input provider.json --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for create
      --input string     promotion provider JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the provider returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion provider](n8n_promotion_provider.md)	 - Manage promotion providers

