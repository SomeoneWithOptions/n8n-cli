## n8n project user role

Manage members' project roles

### Synopsis

Manage the project role of an existing project member. Use 'set' with the
project ID, the user ID, and a project role slug from 'n8n role list'. The role
applies inside this project only; it does not change the user's global role,
which 'n8n user role set' handles.

```
n8n project user role [flags]
```

### Examples

```
  n8n project user role set PROJECT_ID USER_ID project:editor
  n8n project user role set PROJECT_ID USER_ID project:admin --output json
```

### Options

```
  -h, --help   help for role
```

### SEE ALSO

* [n8n project user](n8n_project_user.md)	 - Manage project members
* [n8n project user role set](n8n_project_user_role_set.md)	 - Set a member's project role

