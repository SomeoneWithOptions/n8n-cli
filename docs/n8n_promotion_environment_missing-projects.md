## n8n promotion environment missing-projects

List branch projects missing on this instance (n8n 2.47 and later)

### Synopsis

Read the package at the branch tip of a connected instance-scope apply
environment and list its team projects whose IDs do not exist on this
instance. Projects match by ID, not by name; an ID a personal project
uses counts as existing. Nothing changes.

Create each listed project with its ID, e.g. 'n8n project create NAME
--id PROJECT_ID', then apply its content with the project apply flow.
An empty list means no setup is needed. Find the environment ID with
'n8n promotion environment list --scope instance --direction apply' and
connect it first with 'n8n promotion environment connect'. The route
exists from n8n 2.47. Requires promotions:read.

```
n8n promotion environment missing-projects <environment-id> [flags]
```

### Examples

```
  n8n promotion environment missing-projects ENVIRONMENT_ID
  n8n promotion environment missing-projects ENVIRONMENT_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for missing-projects
      --output string    output format: text or json (JSON is the server result: environmentId, git, and data) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)

