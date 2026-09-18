## n8n promotion project

Manage projects linked to a promotion connection

### Synopsis

Manage which team projects one promotion connection syncs. Start with
'list' to see the linked project IDs, then 'add' a project or
'remove' one.

A project can belong to only one connection, so adding a linked
project elsewhere answers 409. Removing unlinks the project; its
instance content is untouched. Project IDs come from 'n8n project
list'. Adding and listing need gitConnection:read for reads and
gitConnection:manageProjects for writes.

```
n8n promotion project [flags]
```

### Examples

```
  n8n promotion project list CONNECTION_ID
  n8n promotion project add CONNECTION_ID PROJECT_ID
  n8n promotion project remove CONNECTION_ID PROJECT_ID --yes
```

### Options

```
  -h, --help   help for project
```

### SEE ALSO

* [n8n promotion](n8n_promotion.md)	 - Manage promotion providers, connections, and apply/promote sync
* [n8n promotion project add](n8n_promotion_project_add.md)	 - Add a project to a promotion connection
* [n8n promotion project list](n8n_promotion_project_list.md)	 - List projects linked to a promotion connection
* [n8n promotion project remove](n8n_promotion_project_remove.md)	 - Remove a project from a promotion connection

