## n8n project delete

Permanently delete a project

### Synopsis

Permanently delete one project together with the workflows, credentials, and
variables it owns. Members lose access; their user accounts are not deleted.
The endpoint takes no transfer target, so move anything worth keeping first,
for example with 'n8n credential transfer'. This cannot be undone.

Interactive runs ask for confirmation. Non-interactive runs require --yes.
Requires project:delete and a licensed instance.

```
n8n project delete <project-id> [flags]
```

### Examples

```
  n8n project delete PROJECT_ID
  n8n project delete PROJECT_ID --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (JSON is the project object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm permanent project and owned-resource deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n project](n8n_project.md)	 - Manage projects and their members

