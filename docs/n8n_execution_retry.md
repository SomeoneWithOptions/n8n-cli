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

--idempotency-key KEY makes a retry after a timeout or lost response safe:
the same request with the same key returns the first result instead of
running again (auto generates a key and prints it on stderr).

```
n8n execution retry <execution-id> [flags]
```

### Examples

```
  n8n execution retry EXECUTION_ID
  n8n execution retry EXECUTION_ID --latest-workflow
  n8n execution retry EXECUTION_ID --idempotency-key auto --output json
```

### Options

```
      --context string           saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help                     help for retry
      --idempotency-key string   retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)
      --latest-workflow          run the workflow currently saved instead of the definition stored with the original execution (default: stored definition)
      --output string            output format: text or json (JSON is the newly started execution) (default "text")
      --url string               instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions

