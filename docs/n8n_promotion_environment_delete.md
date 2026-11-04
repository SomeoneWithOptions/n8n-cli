## n8n promotion environment delete

Permanently delete a promotion environment

### Synopsis

Permanently delete one promotion environment with its project links and
local checkout. Its provider is kept and nothing in Git changes, but
linked projects fall back to the instance environment, and promote or
apply for that direction stops working until an environment is
created and connected again. This cannot be undone.

To keep the environment and drop only the checkout, use 'n8n promotion
environment disconnect' instead. Interactive runs ask for
confirmation; non-interactive runs require --yes. Requires
gitConnection:delete.

```
n8n promotion environment delete <environment-id> [flags]
```

### Examples

```
  n8n promotion environment delete ENVIRONMENT_ID
  n8n promotion environment delete ENVIRONMENT_ID --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (JSON is the deletion acknowledgement) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm permanent environment, project link, and checkout deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)

