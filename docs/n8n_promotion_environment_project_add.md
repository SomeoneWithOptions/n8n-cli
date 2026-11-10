## n8n promotion environment project add

Link a project to a promotion environment

### Synopsis

Link one team project to one projects-scope promotion environment, so
project-level sync in that direction uses this environment instead of
the instance environment. A project can have one environment for each
direction: adding a project that already has one for this direction
answers 409. Requires promotions:manageProjects
(gitConnection:manageProjects on n8n 2.46).

```
n8n promotion environment project add <environment-id> <project-id> [flags]
```

### Examples

```
  n8n promotion environment project add ENVIRONMENT_ID PROJECT_ID
  n8n promotion environment project add ENVIRONMENT_ID PROJECT_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for add
      --output string    output format: text or json (JSON is the project link returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion environment project](n8n_promotion_environment_project.md)	 - Manage projects linked to a promotion environment

