## n8n evaluation

Run and inspect workflow evaluations

### Synopsis

Manage asynchronous evaluation test runs for workflows configured with an
evaluation trigger. Start with 'list WORKFLOW_ID' to find run IDs, use 'get'
for aggregate metrics, then 'case list' for per-case inputs, outputs and
metrics. Both lists support cursor pagination and --all.

'create' starts a test run and returns immediately in a new or running state;
poll 'get' until completed, error, or cancelled. 'cancel' asks for confirmation
and only applies while a run is cancellable. Creating and cancelling also need
workflow:execute permission in the workflow's project.

```
n8n evaluation [flags]
```

### Examples

```
  n8n evaluation list WORKFLOW_ID
  n8n evaluation create WORKFLOW_ID --output json
  n8n evaluation get WORKFLOW_ID RUN_ID --output json
  n8n evaluation case list WORKFLOW_ID RUN_ID --all --output json
  n8n evaluation cancel WORKFLOW_ID RUN_ID --yes
```

### Options

```
  -h, --help   help for evaluation
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n evaluation cancel](n8n_evaluation_cancel.md)	 - Cancel a new or running evaluation
* [n8n evaluation case](n8n_evaluation_case.md)	 - Inspect cases within an evaluation run
* [n8n evaluation create](n8n_evaluation_create.md)	 - Start an asynchronous workflow evaluation
* [n8n evaluation get](n8n_evaluation_get.md)	 - Show one evaluation run and its aggregate result
* [n8n evaluation list](n8n_evaluation_list.md)	 - List evaluation runs for a workflow

