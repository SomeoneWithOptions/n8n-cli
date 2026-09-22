## n8n evaluation get

Show one evaluation run and its aggregate result

### Synopsis

Show one evaluation run, including asynchronous status, aggregate metrics,
final result, error details and test-case count. New and running runs have no
final result yet; poll this command until completed, error, or cancelled.

Use 'n8n evaluation case list WORKFLOW_ID RUN_ID' for per-case inputs, outputs,
metrics and underlying execution IDs. Requires testRun:read.

```
n8n evaluation get <workflow-id> <run-id> [flags]
```

### Examples

```
  n8n evaluation get WORKFLOW_ID RUN_ID
  n8n evaluation get WORKFLOW_ID RUN_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON preserves workflow-defined metrics and error details) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n evaluation](n8n_evaluation.md)	 - Run and inspect workflow evaluations

