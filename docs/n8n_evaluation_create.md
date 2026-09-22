## n8n evaluation create

Start an asynchronous workflow evaluation

### Synopsis

Start an evaluation test run for a workflow containing a configured evaluation
trigger. The request returns immediately with a run ID and a new or running
status; it does not wait for test cases to finish.

Poll 'n8n evaluation get WORKFLOW_ID RUN_ID' for completion and aggregate
metrics. A conflict can mean the workflow cannot start another test run.
Requires testRun:create plus workflow:execute in the workflow's project; the
instance must license evaluations.

```
n8n evaluation create <workflow-id> [flags]
```

### Examples

```
  n8n evaluation create WORKFLOW_ID
  n8n evaluation create WORKFLOW_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for create
      --output string    output format: text or json (JSON is the accepted run with its initial asynchronous status) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n evaluation](n8n_evaluation.md)	 - Run and inspect workflow evaluations

