## n8n git-connection project remove

Remove a project from a Git connection

### Synopsis

Unlink one project from one Git connection. The project itself and
its instance content are untouched; only the sync link goes away, so
later pushes and pulls skip it. Re-add it later with 'n8n
git-connection project add'.

Interactive runs ask for confirmation; non-interactive runs require
--yes. Requires gitConnection:manageProjects.

```
n8n git-connection project remove <connection-id> <project-id> [flags]
```

### Examples

```
  n8n git-connection project remove CONNECTION_ID PROJECT_ID
  n8n git-connection project remove CONNECTION_ID PROJECT_ID --yes --output json
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

* [n8n git-connection project](n8n_git-connection_project.md)	 - Manage projects linked to a Git connection

