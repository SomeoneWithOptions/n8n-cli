## n8n user create

Invite one or more users

### Synopsis

Create or invite users from a strict JSON array supplied by --input. Each item
requires email and may set a global role slug. Unknown fields, an empty array,
invalid emails, and trailing JSON documents are refused before transport.

n8n returns one result per invitation. Output preserves successful users, invite
URLs, email-delivery state, and individual errors even when a bulk request is
only partly successful. Invite URLs are sensitive. Requires user:create.

```
n8n user create [flags]
```

### Examples

```
  n8n user create --input users.json
  printf '%s\n' '[{"email":"person@example.com","role":"global:member"}]' | n8n user create --input - --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for create
      --input string     user invitation JSON array file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (both preserve every per-user result) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n user](n8n_user.md)	 - Manage instance users and global roles

