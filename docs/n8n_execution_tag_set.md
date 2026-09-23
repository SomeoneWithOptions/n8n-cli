## n8n execution tag set

Replace annotation tags on one execution

### Synopsis

Replace the complete annotation-tag set on one execution with repeated
--tag-id values, or remove all tags with --clear. This is replacement, not
addition: omitted current tags are removed.

List current IDs with 'n8n execution tag list EXECUTION_ID' and available IDs
with 'n8n tag list'. Tag names are not accepted. Requires executionTags:update.

```
n8n execution tag set <execution-id> [flags]
```

### Examples

```
  n8n execution tag set EXECUTION_ID --tag-id TAG_ID
  n8n execution tag set EXECUTION_ID --tag-id TAG_ID --tag-id OTHER_TAG_ID
  n8n execution tag set EXECUTION_ID --clear
  n8n execution tag set EXECUTION_ID --tag-id TAG_ID --output json
```

### Options

```
      --clear                remove every annotation tag from the execution (cannot combine with --tag-id)
      --context string       saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help                 help for set
      --output string        output format: text or json (JSON is the resulting array of tags) (default "text")
      --tag-id stringArray   tag ID the execution should carry, repeatable, from 'n8n tag list' (required unless --clear)
      --url string           instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n execution tag](n8n_execution_tag.md)	 - Read and replace annotation tags on an execution

