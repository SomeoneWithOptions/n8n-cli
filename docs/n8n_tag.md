## n8n tag

Manage workflow tags

### Synopsis

Manage the workflow tags of an n8n instance.

Tags are instance-wide labels attached to workflows. Start with 'list' to find
the tag ID other actions need, then get, create, update or delete. A tag name is
unique per instance: creating or renaming to a name already in use fails with a
conflict.

Deleting a tag also removes it from every workflow that carries it; the workflows
themselves are untouched. Attaching tags to a workflow is not part of this group,
it belongs to the workflow commands.

```
n8n tag [flags]
```

### Examples

```
  n8n tag list
  n8n tag create Production
  n8n tag get TAG_ID
  n8n tag update TAG_ID --name Staging
  n8n tag delete TAG_ID --yes
```

### Options

```
  -h, --help   help for tag
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n tag create](n8n_tag_create.md)	 - Create a tag
* [n8n tag delete](n8n_tag_delete.md)	 - Permanently delete a tag
* [n8n tag get](n8n_tag_get.md)	 - Get one tag
* [n8n tag list](n8n_tag_list.md)	 - List tags
* [n8n tag update](n8n_tag_update.md)	 - Rename a tag

