## n8n workflow unpublish

Take the published version of a workflow offline

### Synopsis

Unpublish one workflow, which n8n v1 called deactivating it. Its triggers stop
firing immediately: schedules stop running and webhook URLs stop responding, so
anything calling them starts failing.

Nothing is deleted and the definition is untouched, so 'n8n workflow publish'
puts it back live. Interactive runs ask for confirmation, because this stops
production automation; non-interactive runs require --yes. Requires
workflow:deactivate.

```
n8n workflow unpublish <workflow-id> [flags]
```

### Examples

```
  n8n workflow unpublish WORKFLOW_ID
  n8n workflow unpublish WORKFLOW_ID --yes
  n8n workflow unpublish WORKFLOW_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for unpublish
      --output string    output format: text or json (JSON is the unpublished workflow) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm taking the workflow offline without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

