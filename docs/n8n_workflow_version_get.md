## n8n workflow version get

Show one stored version of a workflow

### Synopsis

Show one stored version of a workflow: who saved it, when, and the nodes and
connections it holds.

Version IDs come from 'n8n workflow history WORKFLOW_ID'. The response is the
stored definition, not a workflow object, so it cannot be fed to
'n8n workflow update' as it stands. Requires workflow:read.

```
n8n workflow version get <workflow-id> <version-id> [flags]
```

### Examples

```
  n8n workflow version get WORKFLOW_ID VERSION_ID
  n8n workflow version get WORKFLOW_ID VERSION_ID --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the stored version with its nodes) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n workflow version](n8n_workflow_version.md)	 - Read a stored version of a workflow

