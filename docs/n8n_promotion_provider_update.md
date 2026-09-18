## n8n promotion provider update

Update a promotion provider

### Synopsis

Update one promotion provider from strict JSON supplied by --input.
Only the fields present are changed; at least one of name or auth is
required. Auth follows the create shape: ssh-key with an optional key
type, or token with a username and password.

Replacing the credential affects every connection attached to this
provider, so interactive runs ask for confirmation and non-interactive
runs require --yes. Credentials travel only as --input because secrets
are never flags. Requires gitConnection:update.

```
n8n promotion provider update <provider-id> [flags]
```

### Examples

```
  n8n promotion provider update PROVIDER_ID --input provider-update.json
  printf '%s\n' '{"name":"github-new"}' | n8n promotion provider update PROVIDER_ID --input -
  n8n promotion provider update PROVIDER_ID --input provider-update.json --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for update
      --input string     promotion provider JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the provider returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm credential replacement for every attached connection without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion provider](n8n_promotion_provider.md)	 - Manage promotion providers

