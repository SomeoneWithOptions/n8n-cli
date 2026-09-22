## n8n user get

Get one user by ID or email

### Synopsis

Get one user using the exact ID or email accepted by the n8n API. Pass
--include-role to request the user's global role; otherwise the API omits it.

Use 'n8n user list' to discover identifiers. This operation is read-only, may
require instance-owner access, and requires user:read.

```
n8n user get <user-id-or-email> [flags]
```

### Examples

```
  n8n user get USER_ID
  n8n user get person@example.com --include-role
  n8n user get USER_ID --include-role --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --include-role     include user's global role (default: role is omitted by API)
      --output string    output format: text or json (JSON is complete returned user object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n user](n8n_user.md)	 - Manage instance users and global roles

