## n8n execution retry

Retry a failed execution as a new run

### Synopsis

Start a new execution from an existing failed run. By default n8n runs the
workflow definition stored with the original execution, making the retry
reproducible. --latest-workflow instead loads the workflow currently saved on
the instance.

The original execution is retained and the response identifies the new run. A
run that cannot be retried returns a conflict. This starts workflow work and may
repeat its external side effects. Requires execution:retry.

```
n8n execution retry <execution-id> [flags]
```

### Examples

```
  n8n execution retry EXECUTION_ID
  n8n execution retry EXECUTION_ID --latest-workflow
  n8n execution retry EXECUTION_ID --output json
```

### Options

```
      --context string    saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help              help for retry
      --latest-workflow   run the workflow currently saved instead of the definition stored with the original execution (default: stored definition)
      --output string     output format: text or json (JSON is the newly started execution) (default "text")
      --url string        instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions

