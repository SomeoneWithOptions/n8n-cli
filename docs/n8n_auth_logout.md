## n8n auth logout

Delete the stored credential for a context

### Synopsis

Delete the stored credential for a context.

Use this to revoke local access without touching the server. The context
itself is kept, so 'n8n auth login' can restore it. With --purge the context
is removed from config.json too, which is what 'n8n config context delete'
does. The API key stays valid on the n8n instance until revoked there. Use
--yes for scripts.

```
n8n auth logout [flags]
```

### Examples

```
  n8n auth logout
  n8n auth logout --context production --yes
  # Forget the credential and the context itself:
  n8n auth logout --context production --purge --yes
```

### Options

```
      --context string   context to log out of (default: the current context)
  -h, --help             help for logout
      --purge            also remove the context itself from config.json
      --yes              delete without asking (required in non-interactive use)
```

### SEE ALSO

* [n8n auth](n8n_auth.md)	 - Log in to an n8n instance and inspect stored credentials

