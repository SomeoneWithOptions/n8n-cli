## n8n workflow

Manage workflows, their versions and their tags

### Synopsis

Manage n8n workflows: list and read them, create and replace their definitions,
publish and unpublish them, archive, transfer and delete them, and inspect the
version history under the 'version' subgroup and the tags under the 'tag'
subgroup.

Start with 'list' to find workflow IDs. The edit workflow is read, edit, write:
'get ID --output json' writes the definition, 'update ID --input FILE' sends it
back. 'update' is a full replacement, so always start from a fresh 'get'.

Publishing is what n8n v1 called activating: a published workflow runs its
triggers in production. 'archive' is the reversible soft delete and 'delete' is
permanent; delete, unpublish and transfer ask for confirmation.

```
n8n workflow [flags]
```

### Examples

```
  n8n workflow list --active true
  n8n workflow get WORKFLOW_ID --output json > workflow.json
  n8n workflow update WORKFLOW_ID --input workflow.json
  n8n workflow publish WORKFLOW_ID
  n8n workflow history WORKFLOW_ID
  n8n workflow tag set WORKFLOW_ID --tag-id TAG_ID
  n8n workflow delete WORKFLOW_ID --yes
```

### Options

```
  -h, --help   help for workflow
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n workflow archive](n8n_workflow_archive.md)	 - Archive a workflow, the reversible soft delete
* [n8n workflow create](n8n_workflow_create.md)	 - Create a workflow from a definition or as an empty one
* [n8n workflow delete](n8n_workflow_delete.md)	 - Permanently delete a workflow
* [n8n workflow diff](n8n_workflow_diff.md)	 - Compare saved workflows across two contexts
* [n8n workflow get](n8n_workflow_get.md)	 - Show one workflow with its definition
* [n8n workflow history](n8n_workflow_history.md)	 - List the saved versions of a workflow
* [n8n workflow list](n8n_workflow_list.md)	 - List workflows
* [n8n workflow publish](n8n_workflow_publish.md)	 - Publish a workflow so its triggers run
* [n8n workflow tag](n8n_workflow_tag.md)	 - Read and replace the tags of a workflow
* [n8n workflow transfer](n8n_workflow_transfer.md)	 - Move a workflow to another project
* [n8n workflow unarchive](n8n_workflow_unarchive.md)	 - Restore an archived workflow
* [n8n workflow unpublish](n8n_workflow_unpublish.md)	 - Take the published version of a workflow offline
* [n8n workflow update](n8n_workflow_update.md)	 - Replace the definition of a workflow
* [n8n workflow version](n8n_workflow_version.md)	 - Read a stored version of a workflow

