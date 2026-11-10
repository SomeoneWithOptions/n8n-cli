## n8n promotion environment disconnect

Remove the local checkout of a promotion environment

### Synopsis

Remove the local checkout of one promotion environment. The
environment, its credentials, its trusted SSH host keys, and its
project links are kept, but promote and apply through it stop working
until 'connect' runs again.

Interactive runs ask for confirmation; non-interactive runs require
--yes. Requires promotions:connect (gitConnection:clone on n8n 2.46).

```
n8n promotion environment disconnect <environment-id> [flags]
```

### Examples

```
  n8n promotion environment disconnect ENVIRONMENT_ID
  n8n promotion environment disconnect ENVIRONMENT_ID --yes --output json
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

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)

