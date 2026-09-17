## n8n role

Manage global and project roles

### Synopsis

Manage global and project roles on an n8n instance. Start with 'list' to
inspect role slugs, licensing, scopes, and whether each role is built in. Use
'get' for complete details, then create, replace, or delete custom roles.

System roles are immutable. Create and update read strict JSON from --input;
update is a full replacement. Deleting a custom role can optionally reassign
its users first and always requires confirmation.

```
n8n role [flags]
```

### Examples

```
  n8n role list --with-usage-count
  n8n role get global:member
  n8n role create --input role.json
  n8n role update global:custom --input role-update.json
  n8n role delete global:custom --reassign-role global:member --yes
```

### Options

```
  -h, --help   help for role
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n role create](n8n_role_create.md)	 - Create a custom role
* [n8n role delete](n8n_role_delete.md)	 - Permanently delete a custom role
* [n8n role get](n8n_role_get.md)	 - Get one role
* [n8n role list](n8n_role_list.md)	 - List global and project roles
* [n8n role update](n8n_role_update.md)	 - Fully replace a custom role

