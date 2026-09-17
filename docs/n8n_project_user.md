## n8n project user

Manage project members

### Synopsis

Manage which users belong to a project and with which project role. Start with
'list' to see current members and their roles, then add users, change a role, or
remove a member.

User IDs come from 'n8n user list'; project role slugs come from 'n8n role list'
(project roles such as project:viewer, project:editor, project:admin). Listing
members additionally requires the user:list scope; adding, changing and removing
require project:manageMembers.

```
n8n project user [flags]
```

### Examples

```
  n8n project user list PROJECT_ID
  n8n project user add PROJECT_ID --user USER_ID=project:viewer
  n8n project user role set PROJECT_ID USER_ID project:editor
  n8n project user remove PROJECT_ID USER_ID --yes
```

### Options

```
  -h, --help   help for user
```

### SEE ALSO

* [n8n project](n8n_project.md)	 - Manage projects and their members
* [n8n project user add](n8n_project_user_add.md)	 - Add one or more users to a project
* [n8n project user list](n8n_project_user_list.md)	 - List the members of a project
* [n8n project user remove](n8n_project_user_remove.md)	 - Remove a member from a project
* [n8n project user role](n8n_project_user_role.md)	 - Manage members' project roles

