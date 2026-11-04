## n8n promotion environment get

Show one promotion environment

### Synopsis

Show one promotion environment by ID: its name, scope, direction,
remote, provider, settings, local checkout state, and timestamps.
Secrets are never returned. Find the ID with 'n8n promotion environment
list'. The --output json document can be edited and passed to 'update'
as is: its read-only fields are dropped. Requires gitConnection:read.

```
n8n promotion environment get <environment-id> [flags]
```

### Examples

```
  n8n promotion environment get ENVIRONMENT_ID
  n8n promotion environment get ENVIRONMENT_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the environment object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)

