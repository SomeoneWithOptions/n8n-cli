## n8n execution watch

Show executions live as they start and finish

### Synopsis

Poll the execution list and show it live. The n8n API offers no push channel, so
this refetches the newest executions every --interval (default 2s) and shows
what changed. It never writes to the instance and needs only execution:list.

By default the terminal is redrawn each tick as a dashboard of the newest --limit
executions. When stdout is not a terminal no escape codes are written: each
tick prints a timestamp line and the table. --follow instead appends one line
per execution the first time it is seen and again whenever its status changes,
which suits logs and pipes. Filter with --workflow-id, --project-id and
--status exactly as 'n8n execution list' does; without --workflow-id a WORKFLOW
column shows the name, or the ID when the credential lacks workflow:read. The
API's unfiltered list omits running executions, so without --status each tick
also asks for status=running and merges the result (two requests per tick);
pass --status running to see only active runs. DURATION is shown once a run
has finished.

--nodes adds NODES (executed/total) and ERROR columns. They need each run's
detailed data, so every new or changed execution costs one extra request;
finished runs are fetched once and cached. Oversized data the server omits
shows as '-'.

--output json streams NDJSON: one compact object per line, one line per
execution per change, each carrying schemaVersion, type "execution", id,
workflowId, workflowName, status, mode, startedAt, stoppedAt, durationMs, nodes
and error. Diagnostics go to stderr. The command runs until interrupted:
Ctrl+C (or SIGTERM) exits 0. Exit 1 is reserved for transport, credential and
validation errors; a poll that fails mid-run is reported on stderr and retried.
Use 'n8n execution trace' to inspect one run node by node.

```
n8n execution watch [flags]
```

### Examples

```
  n8n execution watch
  n8n execution watch --workflow-id WORKFLOW_ID --nodes
  n8n execution watch --status error --interval 10s
  n8n execution watch --follow
  n8n execution watch --workflow-id WORKFLOW_ID --output json | jq -c 'select(.status=="error")'
```

### Options

```
      --context string       saved context to use (default: the current context; see 'n8n config context list')
      --follow               append a line per new or changed execution instead of redrawing the table (default: redraw)
  -h, --help                 help for watch
      --interval duration    time between polls, e.g. 2s, 500ms is rejected; 1s to 1h (default 2s) (default 2s)
      --limit int            rows in the dashboard, or the initial backfill with --follow, 1 to 250 (default 10) (default 10)
      --nodes                add NODES and ERROR columns from each run's detailed data; one extra request per new or changed execution (default: metadata only)
      --output string        output format: text or json (JSON is NDJSON, one execution object per line per change) (default "text")
      --project-id string    only executions in this project, from 'n8n project list' (default: every accessible project)
      --status string        execution status: canceled, crashed, error, new, running, success, unknown, or waiting (default: every status)
      --url string           instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --workflow-id string   only executions of this workflow, from 'n8n workflow list' (default: every workflow)
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions

