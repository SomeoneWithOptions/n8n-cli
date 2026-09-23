## n8n git-connection project list

List projects linked to a Git connection

### Synopsis

List the team projects added to one Git connection. These are the
projects 'push' exports and 'pull' overwrites. Find the connection ID
with 'n8n git-connection list'. Requires gitConnection:read.

```
n8n git-connection project list <connection-id> [flags]
```

### Examples

```
  n8n git-connection project list CONNECTION_ID
  n8n git-connection project list CONNECTION_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for list
      --output string    output format: text or json (JSON is the projectIds object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n git-connection project](n8n_git-connection_project.md)	 - Manage projects linked to a Git connection

