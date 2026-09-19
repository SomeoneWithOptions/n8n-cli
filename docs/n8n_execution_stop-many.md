## n8n execution stop-many

Stop active executions matching filters

### Synopsis

Cancel queued, running, or waiting executions matching the supplied statuses and
RFC3339 time range within one workflow scope. At least one --status is required.

Scope is explicit: pass --workflow-id for one workflow or --all for every
accessible workflow. Omitting both is rejected so an empty filter can never stop
workflows globally by accident; the literal workflow ID 'all' is rejected, use
--all instead. Stopping is irreversible for each matched run, though a failed
or canceled run may later be retried. Interactive runs ask for confirmation and
non-interactive runs require --yes. Requires execution:stop.

```
n8n execution stop-many [flags]
```

### Examples

```
  n8n execution stop-many --status running --workflow-id WORKFLOW_ID
  n8n execution stop-many --status queued --status waiting --all --yes
  n8n execution stop-many --status running --workflow-id WORKFLOW_ID --started-after 2026-09-17T00:00:00Z --yes --output json
```

### Options

```
      --all                     stop matching executions across every accessible workflow (cannot combine with --workflow-id)
      --context string          saved context to use (default: the current context; see 'n8n config context list')
  -h, --help                    help for stop-many
      --output string           output format: text or json (JSON contains the number stopped) (default "text")
      --started-after string    only executions started after this RFC3339 timestamp, e.g. 2026-09-17T00:00:00Z (default: no lower bound)
      --started-before string   only executions started before this RFC3339 timestamp, e.g. 2026-09-18T00:00:00Z (default: no upper bound)
      --status stringArray      status to stop: queued, running, or waiting; repeatable (at least one required)
      --url string              instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --workflow-id string      only executions of this workflow, from 'n8n workflow list' (required unless --all)
      --yes                     confirm stopping every match without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions

