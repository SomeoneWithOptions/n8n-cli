## n8n project user role set

Set a member's project role

### Synopsis

Change one member's role inside one project. The user must already be a member;
add them first with 'n8n project user add'. Run 'n8n project user list' for the
current members and 'n8n role list' for assignable project role slugs.

This takes effect immediately and changes what the member may do in the project,
but not their global role. Requires project:manageMembers.

```
n8n project user role set <project-id> <user-id> <role-slug> [flags]
```

### Examples

```
  n8n project user role set PROJECT_ID USER_ID project:editor
  n8n project user role set PROJECT_ID USER_ID project:admin --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for set
      --output string    output format: text or json (acknowledgement contains project, user and new role) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n project user role](n8n_project_user_role.md)	 - Manage members' project roles

