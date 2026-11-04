## n8n promotion environment connect

Prepare the local checkout of a promotion environment

### Synopsis

Prepare one promotion environment on the instance that handles the
request; for a Git provider this clones the configured branch into
local storage. Connecting one environment does not make another
ready.

Connecting is safe to repeat and changes no project content by itself,
so it needs no confirmation. Connect again after 'get' reports a
checkout that does not match the environment. Requires
gitConnection:clone.

```
n8n promotion environment connect <environment-id> [flags]
```

### Examples

```
  n8n promotion environment connect ENVIRONMENT_ID
  n8n promotion environment connect ENVIRONMENT_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for connect
      --output string    output format: text or json (JSON is the checkout returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)

