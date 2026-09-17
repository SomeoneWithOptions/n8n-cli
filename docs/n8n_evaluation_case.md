## n8n evaluation case

Inspect cases within an evaluation run

### Synopsis

Inspect per-case results within one evaluation run. Start with 'list' after
finding a run through 'n8n evaluation list'. Case JSON includes workflow-defined
inputs, outputs and metrics plus an underlying execution ID when retained.

Case lists use their own cursor independently from the parent run list. Use
--output json when full case payloads are needed.

```
n8n evaluation case [flags]
```

### Examples

```
  n8n evaluation case list WORKFLOW_ID RUN_ID
  n8n evaluation case list WORKFLOW_ID RUN_ID --all --output json
```

### Options

```
  -h, --help   help for case
```

### SEE ALSO

* [n8n evaluation](n8n_evaluation.md)	 - Run and inspect workflow evaluations
* [n8n evaluation case list](n8n_evaluation_case_list.md)	 - List per-case results for an evaluation run

