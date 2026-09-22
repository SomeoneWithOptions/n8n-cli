## n8n project user remove

Remove a member from a project

### Synopsis

Remove one member from one project. The user account, its personal project, and
everything the project itself owns are untouched; only this membership and the
access it granted go away. Re-add the member later with 'n8n project user add'.

Interactive runs ask for confirmation. Non-interactive runs require --yes.
Requires project:manageMembers.

```
n8n project user remove <project-id> <user-id> [flags]
```

### Examples

```
  n8n project user remove PROJECT_ID USER_ID
  n8n project user remove PROJECT_ID USER_ID --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for remove
      --output string    output format: text or json (acknowledgement contains project and removed user) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm member removal without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n project user](n8n_project_user.md)	 - Manage project members

