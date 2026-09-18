## n8n promotion apply

Apply the Git remote into this instance

### Synopsis

Reset the apply checkout to the branch tip and import projects into
the instance, overwriting to match. The apply checkout must be cloned
first. Preview what is linked with 'n8n promotion project list'.

This rewrites instance content from the remote and cannot be undone:
workflows, folders, credentials, data tables, variables, and tags are
created, updated, or removed to match the branch, and credential gaps
become stubs. Interactive runs ask for confirmation and
non-interactive runs require --yes. Requires gitConnection:pull.

```
n8n promotion apply <connection-id> [flags]
```

### Examples

```
  n8n promotion apply CONNECTION_ID
  n8n promotion apply CONNECTION_ID --yes
  n8n promotion apply CONNECTION_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for apply
      --output string    output format: text or json (JSON is the apply result with counts and commit) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm overwriting instance content from the remote without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion](n8n_promotion.md)	 - Manage promotion providers, connections, and apply/promote sync

