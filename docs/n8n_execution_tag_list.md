## n8n execution tag list

List annotation tags on one execution

### Synopsis

List the complete, unpaginated set of annotation tags attached to one
execution. Run this before 'n8n execution tag set', because setting replaces the
whole set and a partial change must repeat every tag ID to keep. Requires
executionTags:list.

```
n8n execution tag list <execution-id> [flags]
```

### Examples

```
  n8n execution tag list EXECUTION_ID
  n8n execution tag list EXECUTION_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for list
      --output string    output format: text or json (JSON is the complete array of tags) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n execution tag](n8n_execution_tag.md)	 - Read and replace annotation tags on an execution

