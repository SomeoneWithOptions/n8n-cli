## n8n auth

Log in to an n8n instance and inspect stored credentials

### Synopsis

Log in to an n8n instance and inspect stored credentials.

Typical workflow: 'n8n auth login' once per instance, 'n8n auth status'
to inspect what is saved, 'n8n auth status --check' to verify the
credential still works, 'n8n auth logout' to forget it. Run resource
commands only after login succeeds.

```
n8n auth [flags]
```

### Examples

```
  n8n auth login --url https://n8n.example.com
  n8n auth status --check
  n8n auth logout
```

### Options

```
  -h, --help   help for auth
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n auth login](n8n_auth_login.md)	 - Store a credential for an n8n instance
* [n8n auth logout](n8n_auth_logout.md)	 - Delete the stored credential for a context
* [n8n auth status](n8n_auth_status.md)	 - Show the active context, instance and credential state

