## n8n git-connection project

Manage projects linked to a Git connection

### Synopsis

Manage which team projects one Git connection syncs. Start with
'list' to see the linked project IDs, then 'add' a project or
'remove' one.

A project can belong to only one connection, so adding a linked
project elsewhere answers 409. Removing unlinks the project; its
instance content is untouched. Project IDs come from 'n8n project
list'. Adding and listing need gitConnection:read for reads and
gitConnection:manageProjects for writes.

```
n8n git-connection project [flags]
```

### Examples

```
  n8n git-connection project list CONNECTION_ID
  n8n git-connection project add CONNECTION_ID PROJECT_ID
  n8n git-connection project remove CONNECTION_ID PROJECT_ID --yes
```

### Options

```
  -h, --help   help for project
```

### SEE ALSO

* [n8n git-connection](n8n_git-connection.md)	 - Manage the Git connection, its projects, and push/pull sync
* [n8n git-connection project add](n8n_git-connection_project_add.md)	 - Add a project to a Git connection
* [n8n git-connection project list](n8n_git-connection_project_list.md)	 - List projects linked to a Git connection
* [n8n git-connection project remove](n8n_git-connection_project_remove.md)	 - Remove a project from a Git connection

