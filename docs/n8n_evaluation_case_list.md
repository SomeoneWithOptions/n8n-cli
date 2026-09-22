## n8n evaluation case list

List per-case results for an evaluation run

### Synopsis

List one evaluation run's per-case statuses, timestamps and retained execution
IDs. JSON output additionally preserves each case's workflow-defined inputs,
outputs, metrics and error details.

Pass nextCursor to --cursor, or use --all to follow every case page up to 10,000
cases. This pagination is independent from 'n8n evaluation list'. Requires
testRun:read and access to the workflow's project.

```
n8n evaluation case list <workflow-id> <run-id> [flags]
```

### Examples

```
  n8n evaluation case list WORKFLOW_ID RUN_ID
  n8n evaluation case list WORKFLOW_ID RUN_ID --limit 50 --cursor NEXT_CURSOR
  n8n evaluation case list WORKFLOW_ID RUN_ID --all --output json
```

### Options

```
      --all              follow every test-case page instead of one (maximum 10,000 cases)
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous case list (default: first page)
  -h, --help             help for list
      --limit int        test cases per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON includes case inputs, outputs, metrics and errors) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n evaluation case](n8n_evaluation_case.md)	 - Inspect cases within an evaluation run

