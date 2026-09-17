## n8n workflow tag list

List the tags attached to a workflow

### Synopsis

List the tags one workflow carries, with their IDs and names. The response is
the whole set, not a page: workflow tags are not paginated.

Run this before 'n8n workflow tag set', because setting replaces the whole set
and the current IDs are what a partial change has to repeat. Requires
workflowTags:list.

```
n8n workflow tag list <workflow-id> [flags]
```

### Examples

```
  n8n workflow tag list WORKFLOW_ID
  n8n workflow tag list WORKFLOW_ID --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for list
      --output string    output format: text or json (JSON is the array of tags) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n workflow tag](n8n_workflow_tag.md)	 - Read and replace the tags of a workflow

