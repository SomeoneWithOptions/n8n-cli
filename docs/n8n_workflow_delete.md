## n8n workflow delete

Permanently delete a workflow

### Synopsis

Delete one workflow for good, together with its version history. This cannot be
undone, and the executions it produced lose the definition they refer to.

Prefer 'n8n workflow archive' when the workflow may be wanted later: archiving
hides it and is reversible with 'n8n workflow unarchive'. Back the definition
up first with 'n8n workflow get ID --output json > workflow.json'.

Interactive runs ask for confirmation. Non-interactive runs require --yes.
Requires workflow:delete.

```
n8n workflow delete <workflow-id> [flags]
```

### Examples

```
  n8n workflow delete WORKFLOW_ID
  n8n workflow delete WORKFLOW_ID --yes
  n8n workflow delete WORKFLOW_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (JSON is the workflow as the server last saw it) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm the deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

