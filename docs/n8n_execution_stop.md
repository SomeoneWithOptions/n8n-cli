## n8n execution stop

Stop one queued, running, or waiting execution

### Synopsis

Cancel one queued, running, or waiting execution. The current run cannot resume;
use 'n8n execution retry ID' afterwards when it should run again as a new
execution. A run already finished or otherwise not stoppable returns a conflict.

Interactive runs ask for confirmation and non-interactive runs require --yes.
The workflow definition is untouched. Requires execution:stop.

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
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for stop
      --output string    output format: text or json (JSON is the resulting stop state) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm canceling the execution without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions

