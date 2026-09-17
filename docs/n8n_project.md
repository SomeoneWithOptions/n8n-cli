## n8n project

Manage projects and their members

### Synopsis

Manage projects on an n8n instance. Start with 'list' to find project IDs,
then create, rename, or delete a project. Membership lives under the 'user'
subgroup: list members, add them with a project role, change a role, or remove
a member.

Projects are a licensed n8n feature: an instance without the entitlement answers
403 for every command here. Deleting a project also deletes the workflows,
credentials, and variables it owns, so it always requires confirmation.

```
n8n project [flags]
```

### Examples

```
  n8n project list
  n8n project create "Billing automation"
  n8n project update PROJECT_ID --name "Billing"
  n8n project user list PROJECT_ID
  n8n project user add PROJECT_ID --user USER_ID=project:viewer
  n8n project delete PROJECT_ID --yes
```

### Options

```
  -h, --help   help for project
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n project create](n8n_project_create.md)	 - Create a project
* [n8n project delete](n8n_project_delete.md)	 - Permanently delete a project
* [n8n project list](n8n_project_list.md)	 - List projects
* [n8n project update](n8n_project_update.md)	 - Rename a project
* [n8n project user](n8n_project_user.md)	 - Manage project members

