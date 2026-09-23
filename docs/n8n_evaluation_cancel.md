## n8n evaluation cancel

Cancel a new or running evaluation

### Synopsis

Request cancellation of one new or running evaluation. Work already performed
by its cases is not rolled back, and the same run cannot resume. A run that
already reached completed, error, or cancelled returns a conflict.

Interactive runs ask for confirmation; non-interactive runs require --yes. Use
'n8n evaluation get WORKFLOW_ID RUN_ID' first to inspect current state.
Requires testRun:cancel plus workflow:execute in the workflow's project.

```
n8n evaluation cancel <workflow-id> <run-id> [flags]
```

### Examples

```
  n8n evaluation cancel WORKFLOW_ID RUN_ID
  n8n evaluation cancel WORKFLOW_ID RUN_ID --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for cancel
      --output string    output format: text or json (JSON is the cancellation acknowledgement) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm cancellation without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n evaluation](n8n_evaluation.md)	 - Run and inspect workflow evaluations

