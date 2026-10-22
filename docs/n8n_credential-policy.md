## n8n credential-policy

Manage credential type availability policies

### Synopsis

Manage instance and project credential type policies. Start with 'instance get', then inspect 'attachment get' before editing scopes with shared documents. GET returns composed rules; scope set replaces only the scope's own document and default. Documents are reusable rule lists. Floor attachments run first, then priority ascending within each group. The first matching rule wins. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy [flags]
```

### Examples

```
  n8n credential-policy instance get
  n8n credential-policy document list
```

### Options

```
  -h, --help   help for credential-policy
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n credential-policy attachment](n8n_credential-policy_attachment.md)	 - Manage attached credential type policy documents
* [n8n credential-policy document](n8n_credential-policy_document.md)	 - Manage reusable credential type policy documents
* [n8n credential-policy instance](n8n_credential-policy_instance.md)	 - Manage instance credential type policies
* [n8n credential-policy project](n8n_credential-policy_project.md)	 - Manage project credential type policies

