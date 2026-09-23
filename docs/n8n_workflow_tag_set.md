## n8n workflow tag set

Replace the tags of a workflow

### Synopsis

Replace the whole set of tags on one workflow with the tag IDs given by
--tag-id, or remove every tag with --clear.

This is a replacement, not an addition: a tag the workflow carries today and
that is not repeated here is removed from it. To add one tag, run
'n8n workflow tag list WORKFLOW_ID' first and pass the existing IDs along with
the new one. The API takes tag IDs, never tag names; find them with
'n8n tag list'. Requires workflowTags:update.

```
n8n workflow tag set <workflow-id> [flags]
```

### Examples

```
  n8n workflow tag set WORKFLOW_ID --tag-id TAG_ID
  n8n workflow tag set WORKFLOW_ID --tag-id TAG_ID --tag-id OTHER_TAG_ID
  n8n workflow tag set WORKFLOW_ID --clear
  n8n workflow tag set WORKFLOW_ID --tag-id TAG_ID --output json
```

### Options

```
      --clear                remove every tag from the workflow (cannot combine with --tag-id)
      --context string       saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help                 help for set
      --output string        output format: text or json (JSON is the resulting array of tags) (default "text")
      --tag-id stringArray   tag ID the workflow should carry, repeatable, from 'n8n tag list' (required unless --clear)
      --url string           instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n workflow tag](n8n_workflow_tag.md)	 - Read and replace the tags of a workflow

