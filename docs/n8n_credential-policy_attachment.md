## n8n credential-policy attachment

Manage attached credential type policy documents

### Synopsis

Inspect or replace a scope's complete attachment list. Start with 'get'. Obtain SCOPE_ID from 'instance get' or 'project get PROJECT_ID' -> scopeId; it is null until that scope is first written with set. Requires instance-level access even for project scopes. Floor attachments evaluate first, then priority ascending within each group. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy attachment [flags]
```

### Examples

```
  n8n credential-policy instance get --output json
  n8n credential-policy attachment get SCOPE_ID
```

### Options

```
  -h, --help   help for attachment
```

### SEE ALSO

* [n8n credential-policy](n8n_credential-policy.md)	 - Manage credential type availability policies
* [n8n credential-policy attachment get](n8n_credential-policy_attachment_get.md)	 - Get credential type policy attachments
* [n8n credential-policy attachment set](n8n_credential-policy_attachment_set.md)	 - Replace credential type policy attachments

