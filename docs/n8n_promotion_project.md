## n8n promotion project

Manage projects linked to a promotion connection

### Synopsis

Manage which team projects a projects-scope promotion connection
syncs; a project's own connection takes precedence over the instance
connection for project-level sync. Start with
'list' to see the linked project IDs, then 'add' a project or
'remove' one.

A project can belong to only one connection, so adding a linked
project elsewhere answers 409. Removing unlinks the project; its
instance content is untouched. Project IDs come from 'n8n project
list'. Adding and listing need gitConnection:read for reads and
gitConnection:manageProjects for writes. For selected workflow sync,
start with changes PROJECT_ID apply|promote, then apply or promote.
Selection commands take a project ID first and resolve its own connection
before falling back to the instance connection.

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
* [n8n promotion project apply](n8n_promotion_project_apply.md)	 - Apply selected workflow changes from a project's branch
* [n8n promotion project apply-continue](n8n_promotion_project_apply-continue.md)	 - Continue a reviewed project apply after binding setup
* [n8n promotion project changes](n8n_promotion_project_changes.md)	 - List workflow changes for a project's promotion branch
* [n8n promotion project list](n8n_promotion_project_list.md)	 - List projects linked to a promotion connection
* [n8n promotion project promote](n8n_promotion_project_promote.md)	 - Promote selected project workflows to the remote branch
* [n8n promotion project remove](n8n_promotion_project_remove.md)	 - Remove a project from a promotion connection

