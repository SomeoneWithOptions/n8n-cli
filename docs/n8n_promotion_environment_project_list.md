## n8n promotion environment project list

List projects linked to a promotion environment

### Synopsis

List the team projects linked to one projects-scope promotion
environment. Find the environment ID with 'n8n promotion environment
list --scope projects'. Requires gitConnection:read.

```
n8n promotion environment project list <environment-id> [flags]
```

### Examples

```
  n8n promotion environment project list ENVIRONMENT_ID
  n8n promotion environment project list ENVIRONMENT_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for list
      --output string    output format: text or json (JSON is the projectIds object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion environment project](n8n_promotion_environment_project.md)	 - Manage projects linked to a promotion environment

