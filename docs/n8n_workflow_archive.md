## n8n workflow archive

Archive a workflow, the reversible soft delete

### Synopsis

Archive one workflow. The workflow is hidden from the normal workflow list and
stops being offered for editing, but nothing is destroyed: its definition,
version history and executions stay, and 'n8n workflow unarchive' brings it
back.

Archiving is idempotent: archiving an already archived workflow returns it
unchanged. A published workflow should be unpublished first, so its triggers
stop running. Requires workflow:delete.

```
n8n workflow archive <workflow-id> [flags]
```

### Examples

```
  n8n workflow archive WORKFLOW_ID
  n8n workflow archive WORKFLOW_ID --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for archive
      --output string    output format: text or json (JSON is the archived workflow) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

