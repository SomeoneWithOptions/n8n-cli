## n8n execution stop

Stop one queued, running, or waiting execution

### Synopsis

Cancel one queued, running, or waiting execution. The current run cannot resume;
use 'n8n execution retry ID' afterwards when it should run again as a new
execution. A run already finished or otherwise not stoppable returns a conflict.

Interactive runs ask for confirmation and non-interactive runs require --yes.
The workflow definition is untouched. Requires execution:stop.

--idempotency-key KEY makes a retry after a timeout or lost response safe:
the same request with the same key returns the first result instead of
running again (auto generates a key and prints it on stderr).

```
n8n execution stop <execution-id> [flags]
```

### Examples

```
  n8n execution stop EXECUTION_ID
  n8n execution stop EXECUTION_ID --yes
  n8n execution stop EXECUTION_ID --yes --output json
```

### Options

```
      --context string           saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help                     help for stop
      --idempotency-key string   retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)
      --output string            output format: text or json (JSON is the resulting stop state) (default "text")
      --url string               instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes                      confirm canceling the execution without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions

