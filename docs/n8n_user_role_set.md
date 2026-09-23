## n8n user role set

Set a user's global role

### Synopsis

Change one user's global role using the exact user ID or email accepted by the
API. The role slug must identify an assignable global role; run 'n8n role list'
to inspect licensing and available slugs. This can immediately change the user's
instance permissions. Requires user:changeRole.

```
n8n user role set <user-id-or-email> <role-slug> [flags]
```

### Examples

```
  n8n user role set USER_ID global:member
  n8n user role set person@example.com global:admin --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for set
      --output string    output format: text or json (acknowledgement contains user identifier and new role) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n user role](n8n_user_role.md)	 - Manage users' global roles

