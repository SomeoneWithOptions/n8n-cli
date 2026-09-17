## n8n execution tag

Read and replace annotation tags on an execution

### Synopsis

Read and replace the annotation tags attached to an execution. Tags themselves
are managed with 'n8n tag'; this subgroup only controls which existing tag IDs
annotate one execution.

Start with 'list', then use 'set'. Setting is replacement, not addition: every
current tag that is not repeated is removed. Use --clear to remove all tags.

```
n8n execution tag [flags]
```

### Examples

```
  n8n execution tag list EXECUTION_ID
  n8n tag list
  n8n execution tag set EXECUTION_ID --tag-id TAG_ID
  n8n execution tag set EXECUTION_ID --clear
```

### Options

```
  -h, --help   help for tag
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions
* [n8n execution tag list](n8n_execution_tag_list.md)	 - List annotation tags on one execution
* [n8n execution tag set](n8n_execution_tag_set.md)	 - Replace annotation tags on one execution

