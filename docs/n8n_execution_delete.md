## n8n execution delete

Permanently delete one stored execution

### Synopsis

Permanently delete one execution record and its stored run data. The workflow
it ran is not changed or deleted, but this execution can no longer be inspected,
retried, or used for debugging. This cannot be undone.

Interactive runs ask for confirmation. Non-interactive runs require --yes. Use
'n8n execution get ID --output json' first when a record must be retained.
Requires execution:delete.

```
n8n execution delete <execution-id> [flags]
```

### Examples

```
  n8n execution delete EXECUTION_ID
  n8n execution delete EXECUTION_ID --yes
  n8n execution delete EXECUTION_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (JSON is the deleted execution as last stored) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm permanent deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions

