## n8n user role

Manage users' global roles

### Synopsis

Manage global role assignments for instance users. Use 'set' with a user ID
or email and a role slug discovered through 'n8n role list'. Role changes can
alter instance-wide permissions and may require owner access or licensing.

```
n8n user role [flags]
```

### Examples

```
  n8n user role set USER_ID global:member
  n8n user role set person@example.com global:admin --output json
```

### Options

```
  -h, --help   help for role
```

### SEE ALSO

* [n8n user](n8n_user.md)	 - Manage instance users and global roles
* [n8n user role set](n8n_user_role_set.md)	 - Set a user's global role

