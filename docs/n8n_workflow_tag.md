## n8n workflow tag

Read and replace the tags of a workflow

### Synopsis

Read and replace the tags attached to one workflow. Tags themselves are managed
with 'n8n tag', which is where tag IDs come from; this group only decides which
of them a workflow carries.

Start with 'list' to see the current tags, then 'set' to replace them. 'set' is
a replacement, not an addition: whatever is not passed is removed from the
workflow.

```
n8n workflow tag [flags]
```

### Examples

```
  n8n workflow tag list WORKFLOW_ID
  n8n tag list
  n8n workflow tag set WORKFLOW_ID --tag-id TAG_ID --tag-id OTHER_TAG_ID
  n8n workflow tag set WORKFLOW_ID --clear
```

### Options

```
  -h, --help   help for tag
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags
* [n8n workflow tag list](n8n_workflow_tag_list.md)	 - List the tags attached to a workflow
* [n8n workflow tag set](n8n_workflow_tag_set.md)	 - Replace the tags of a workflow

