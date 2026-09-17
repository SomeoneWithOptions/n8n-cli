## n8n user

Manage instance users and global roles

### Synopsis

Manage users on an n8n instance. Start with 'list' to find user IDs and
emails, use 'get' for one account, then invite users, change a global role,
or permanently delete an account. List and get may require instance-owner
access.

Create accepts a strict JSON array and preserves every per-user success or
error. Role changes and deletion accept the exact user ID or email allowed by
the API. Deletion always requires confirmation.

```
n8n user [flags]
```

### Examples

```
  n8n user list --include-role
  n8n user get person@example.com --include-role
  n8n user create --input users.json
  n8n user role set USER_ID global:member
  n8n user delete USER_ID --yes
```

### Options

```
  -h, --help   help for user
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n user create](n8n_user_create.md)	 - Invite one or more users
* [n8n user delete](n8n_user_delete.md)	 - Permanently delete a user
* [n8n user get](n8n_user_get.md)	 - Get one user by ID or email
* [n8n user list](n8n_user_list.md)	 - List instance or project users
* [n8n user role](n8n_user_role.md)	 - Manage users' global roles

