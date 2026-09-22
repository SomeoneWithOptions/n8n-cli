## n8n project update

Rename a project

### Synopsis

Rename one project. The API replaces the whole project, but name is its only
writable field, so --name is the full replacement document and nothing else is
lost: the ID, type, members, and owned resources all stay as they are.

No read-modify-write step is needed. Find the ID with 'n8n project list'.
Requires project:update and a licensed instance.

```
n8n project update <project-id> [flags]
```

### Examples

```
  n8n project update PROJECT_ID --name "Billing"
  n8n project update PROJECT_ID --name "Customer facing" --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for update
      --name string      new project name, replacing the current one (required)
      --output string    output format: text or json (JSON is the project object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n project](n8n_project.md)	 - Manage projects and their members

