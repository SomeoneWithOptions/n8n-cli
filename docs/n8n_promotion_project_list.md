## n8n promotion project list

List projects linked to a promotion connection

### Synopsis

List the team projects added to one promotion connection. These are
the projects 'promote' exports and 'apply' overwrites. Find the
connection ID with 'n8n promotion connection list'. Requires
gitConnection:read.

```
n8n promotion project list <connection-id> [flags]
```

### Examples

```
  n8n promotion project list CONNECTION_ID
  n8n promotion project list CONNECTION_ID --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for list
      --output string    output format: text or json (JSON is the projectIds object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n promotion project](n8n_promotion_project.md)	 - Manage projects linked to a promotion connection

