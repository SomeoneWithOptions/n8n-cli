## n8n promotion environment project remove

Unlink a project from a promotion environment

### Synopsis

Unlink one project from one promotion environment. The project itself
and its instance content are untouched; only the sync link goes away,
so project-level sync in that direction falls back to the instance
environment. Re-link it later with 'n8n promotion environment project
add'.

Interactive runs ask for confirmation; non-interactive runs require
--yes. Requires gitConnection:manageProjects.

```
n8n promotion environment project remove <environment-id> <project-id> [flags]
```

### Examples

```
  n8n promotion environment project remove ENVIRONMENT_ID PROJECT_ID
  n8n promotion environment project remove ENVIRONMENT_ID PROJECT_ID --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for remove
      --output string    output format: text or json (JSON is the unlinking acknowledgement) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm project unlinking without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion environment project](n8n_promotion_environment_project.md)	 - Manage projects linked to a promotion environment

