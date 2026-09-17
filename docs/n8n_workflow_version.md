## n8n workflow version

Read a stored version of a workflow

### Synopsis

Read one stored version of a workflow definition. Versions are listed by
'n8n workflow history', which reports the version IDs this group takes.

Start with 'n8n workflow history WORKFLOW_ID', then 'version get' to see what
a version contains. Publishing an older version is 'n8n workflow publish
--version-id'; restoring one as the current draft means sending its nodes and
connections through 'n8n workflow update'.

```
n8n workflow version [flags]
```

### Examples

```
  n8n workflow history WORKFLOW_ID
  n8n workflow version get WORKFLOW_ID VERSION_ID
  n8n workflow version get WORKFLOW_ID VERSION_ID --output json
```

### Options

```
  -h, --help   help for version
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags
* [n8n workflow version get](n8n_workflow_version_get.md)	 - Show one stored version of a workflow

