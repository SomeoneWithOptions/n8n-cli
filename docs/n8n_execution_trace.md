## n8n execution trace

Show one execution node by node, optionally as it runs

### Synopsis

Print the per-node trace of one execution: each node run in order with its
status, duration and error class and message. This is n8n's stored run record,
not console output. It never writes to the instance and always requests the
run's detailed data, so it needs execution:read; --workflow-id also needs
execution:list, and workflow names need workflow:read (the ID is shown
otherwise).

Pass an execution ID, or --workflow-id to trace that workflow's most recent run.
With --follow the command polls every --interval (default 2s; the API offers no
push) and prints node runs as they appear, exiting when the run reaches a
terminal status (success, error, crashed, canceled), so command termination is a
usable 'the run finished' signal. '--workflow-id ID --follow' attaches to the
active run of that workflow, or waits for the next one to start: start the
command, then trigger the workflow. Active runs are found through a
status=running query, because the API's unfiltered list omits them. Node
markers are [ok], [fail], [wait], [run] and [stop].

Node data may only appear when the run ends. n8n stores per-node data during a
run only when the workflow's 'Save execution progress' setting is on; otherwise
a followed run shows nothing until it finishes and the trace then prints in one
block. --follow checks that setting first and warns on stderr; the check is
advisory (--no-preflight skips it). Oversized data the server omits is reported
as such; pass --ignore-data-size-limit to fetch it anyway.

Exit codes: 0 once the trace is printed, whatever the run's outcome, and also on
Ctrl+C, including while waiting for a run. 1 for transport, credential,
not-found and validation errors, or when --fail-on-error is set and the run
ended error or crashed. --output json prints one indented object; with
--follow it streams NDJSON lines of type header, node and result, each with
schemaVersion. Warnings stay on stderr so stdout is pure data.

```
n8n execution trace [execution-id] [flags]
```

### Examples

```
  n8n execution trace EXECUTION_ID
  n8n execution trace EXECUTION_ID --verbose
  n8n execution trace --workflow-id WORKFLOW_ID
  n8n execution trace --workflow-id WORKFLOW_ID --follow
  n8n execution trace EXECUTION_ID --output json
  n8n execution trace --workflow-id WORKFLOW_ID --follow --fail-on-error --output json
```

### Options

```
      --context string                 saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --fail-on-error                  exit 1 when the traced run ends error or crashed (default: exit 0 whatever the outcome)
      --follow                         poll until the run reaches a terminal status, printing node runs as they appear (default: print once and exit)
  -h, --help                           help for trace
      --ignore-data-size-limit         return detailed data even when it exceeds the instance display-size limit (default: omit oversized data)
      --interval duration              time between polls with --follow, e.g. 2s; 1s to 1h (default 2s) (default 2s)
      --no-preflight                   skip the advisory check of the workflow's 'Save execution progress' setting before --follow (default: check and warn)
      --output string                  output format: text or json (JSON is one indented object, or NDJSON lines with --follow) (default "text")
      --redact-execution-data string   detailed-data redaction: true always redacts, false reveals and needs execution:reveal (default: workflow policy)
      --url string                     instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --verbose                        also print each node run's input/output data as indented JSON (default: status, duration and error only)
      --workflow-id string             trace this workflow's most recent run, or with --follow its active or next run; cannot be combined with an execution ID
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions

