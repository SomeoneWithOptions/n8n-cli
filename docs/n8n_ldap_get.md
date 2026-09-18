## n8n ldap get

Get LDAP configuration

### Synopsis

Get every LDAP setting exposed by n8n. The binding admin password is never
returned: bindingAdminPassword is empty when unset or n8n's blanking placeholder
when configured. Keep that placeholder unchanged in a GET-edit-PUT workflow to
preserve the stored password. Text output only reports password status.

Requires ldap:manage and the LDAP license.

```
n8n ldap get [flags]
```

### Examples

```
  n8n ldap get
  n8n ldap get --output json
  umask 077 && n8n ldap get --output json > ldap.json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON contains only a password placeholder, never the stored password) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n ldap](n8n_ldap.md)	 - Manage licensed LDAP settings and synchronization

