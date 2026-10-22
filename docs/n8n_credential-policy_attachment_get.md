## n8n credential-policy attachment get

Get credential type policy attachments

### Synopsis

Read a scope's attachments and version in evaluation order: floor first, then priority ascending within each group. Get SCOPE_ID from 'instance get' or 'project get PROJECT_ID' -> scopeId; it is null until the scope is first written. Requires instance-level access even for project scopes. Use the returned version with 'attachment set'. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy attachment get <scope-id> [flags]
```

### Examples

```
  n8n credential-policy attachment get SCOPE_ID
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (default: text; JSON contains the full response) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n credential-policy attachment](n8n_credential-policy_attachment.md)	 - Manage attached credential type policy documents

