## n8n user delete

Permanently delete a user

### Synopsis

Permanently delete one user using the exact ID or email accepted by the API.
This removes the account and can delete resources owned by its personal project;
the documented endpoint provides no ownership-transfer option. It cannot be
undone and the instance owner cannot be deleted.

Interactive runs ask for confirmation. Non-interactive runs require --yes.
Requires user:delete.

```
n8n user delete <user-id-or-email> [flags]
```

### Examples

```
  n8n user delete USER_ID
  n8n user delete person@example.com --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (acknowledgement contains only deleted user identifier) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm permanent user and owned-resource deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n user](n8n_user.md)	 - Manage instance users and global roles

