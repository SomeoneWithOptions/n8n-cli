## n8n workflow get

Show one workflow with its definition

### Synopsis

Show one workflow: its state, its tags and its full node definition.

'--output json' is the faithful copy of the workflow and the first half of the
edit workflow: redirect it to a file, edit the file, then send it back with
'n8n workflow update ID --input FILE'. Text output summarizes instead, listing
the nodes rather than their parameters.

Use --exclude-pinned-data when the pinned sample data is not wanted; note that
a document saved that way no longer carries it, so updating from it clears the
pinned data on the server. Requires workflow:read.

```
n8n workflow get <workflow-id> [flags]
```

### Examples

```
  n8n workflow get WORKFLOW_ID
  n8n workflow get WORKFLOW_ID --output json > workflow.json
  n8n workflow get WORKFLOW_ID --exclude-pinned-data --output json
```

### Options

```
      --context string        saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --exclude-pinned-data   leave pinned sample data out of the response (default: include it)
  -h, --help                  help for get
      --output string         output format: text or json (JSON is the whole workflow, suitable for --input) (default "text")
      --url string            instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

