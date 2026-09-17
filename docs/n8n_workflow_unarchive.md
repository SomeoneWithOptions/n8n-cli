## n8n workflow unarchive

Restore an archived workflow

### Synopsis

Restore one archived workflow, undoing 'n8n workflow archive'. The workflow
becomes visible and editable again.

Restoring does not publish it: a workflow that was published before archiving
comes back unpublished, so run 'n8n workflow publish' when its triggers should
run again. Requires workflow:delete.

```
n8n workflow unarchive <workflow-id> [flags]
```

### Examples

```
  n8n workflow unarchive WORKFLOW_ID
  n8n workflow unarchive WORKFLOW_ID --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for unarchive
      --output string    output format: text or json (JSON is the restored workflow) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

