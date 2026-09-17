## n8n execution stop-many

Stop active executions matching filters

### Synopsis

Cancel queued, running, or waiting executions matching the supplied statuses and
optional workflow and RFC3339 time range. At least one --status is required.

Without --workflow-id the operation spans every accessible workflow; the literal
workflow ID 'all' has the same effect. Stopping is irreversible for each matched
run, though a failed or canceled run may later be retried. Interactive runs ask
for confirmation and non-interactive runs require --yes. Requires execution:stop.

```
n8n execution stop-many [flags]
```

### Examples

```
  n8n execution stop-many --status running --workflow-id WORKFLOW_ID
  n8n execution stop-many --status queued --status waiting --yes
  n8n execution stop-many --status running --started-after 2026-09-17T00:00:00Z --yes --output json
```

### Options

```
      --context string          saved context to use (default: the current context; see 'n8n config context list')
  -h, --help                    help for stop-many
      --output string           output format: text or json (JSON contains the number stopped) (default "text")
      --started-after string    only executions started after this RFC3339 timestamp, e.g. 2026-09-17T00:00:00Z (default: no lower bound)
      --started-before string   only executions started before this RFC3339 timestamp, e.g. 2026-09-18T00:00:00Z (default: no upper bound)
      --status stringArray      status to stop: queued, running, or waiting; repeatable (at least one required)
      --url string              instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --workflow-id string      only executions of this workflow; omit or pass all for every accessible workflow (default: all workflows)
      --yes                     confirm stopping every match without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions

