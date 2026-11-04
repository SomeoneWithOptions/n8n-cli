## n8n promotion environment project

Manage projects linked to a promotion environment

### Synopsis

Manage which team projects a projects-scope promotion environment
syncs. A project can have one environment for each direction; a
direction without a project environment uses the instance environment.
Start with 'list' to see the linked project IDs, then 'add' a project
or 'remove' one.

Removing unlinks the project; its instance content is untouched.
Project IDs come from 'n8n project list'. Listing needs
gitConnection:read; adding and removing need
gitConnection:manageProjects.

```
n8n promotion environment project [flags]
```

### Examples

```
  n8n promotion environment project list ENVIRONMENT_ID
  n8n promotion environment project add ENVIRONMENT_ID PROJECT_ID
  n8n promotion environment project remove ENVIRONMENT_ID PROJECT_ID --yes
```

### Options

```
  -h, --help   help for project
```

### SEE ALSO

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)
* [n8n promotion environment project add](n8n_promotion_environment_project_add.md)	 - Link a project to a promotion environment
* [n8n promotion environment project list](n8n_promotion_environment_project_list.md)	 - List projects linked to a promotion environment
* [n8n promotion environment project remove](n8n_promotion_environment_project_remove.md)	 - Unlink a project from a promotion environment

