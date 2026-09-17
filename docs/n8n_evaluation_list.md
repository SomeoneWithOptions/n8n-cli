## n8n evaluation list

List evaluation runs for a workflow

### Synopsis

List asynchronous evaluation test runs for one workflow. Filter by new,
running, completed, error, or cancelled status. Pass nextCursor to --cursor,
or use --all to follow every page up to 10,000 runs.

Use 'n8n evaluation get WORKFLOW_ID RUN_ID' for aggregate metrics and final
result, then 'n8n evaluation case list' for individual cases. Requires
testRun:list and access to the workflow's project.

```
n8n evaluation list <workflow-id> [flags]
```

### Examples

```
  n8n evaluation list WORKFLOW_ID
  n8n evaluation list WORKFLOW_ID --status running --limit 50
  n8n evaluation list WORKFLOW_ID --all --output json
```

### Options

```
      --all              follow every run page instead of one (maximum 10,000 runs)
      --context string   saved context to use (default: the current context; see 'n8n config context list')
      --cursor string    pagination cursor returned by a previous run list (default: first page)
  -h, --help             help for list
      --limit int        evaluation runs per API page, 1 to 250 (default: server default of 100)
      --output string    output format: text or json (JSON is a page object with data and nextCursor) (default "text")
      --status string    run status: new, running, completed, error, or cancelled (default: every status)
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n evaluation](n8n_evaluation.md)	 - Run and inspect workflow evaluations

