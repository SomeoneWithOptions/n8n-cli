## n8n node-policy attachment

Manage attached node type policy documents

### Synopsis

Inspect or replace a scope's complete attachment list. Start with 'get'. Obtain SCOPE_ID from 'instance get' or 'project get PROJECT_ID' -> scopeId; it is null until that scope is first written with set. Requires instance-level access even for project scopes. Floor attachments evaluate first, then priority ascending within each group. Preview API: breaking changes may occur. Requires nodeTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n node-policy attachment [flags]
```

### Examples

```
  n8n node-policy instance get --output json
  n8n node-policy attachment get SCOPE_ID
```

### Options

```
  -h, --help   help for attachment
```

### SEE ALSO

* [n8n node-policy](n8n_node-policy.md)	 - Manage node type availability policies
* [n8n node-policy attachment get](n8n_node-policy_attachment_get.md)	 - Get node type policy attachments
* [n8n node-policy attachment set](n8n_node-policy_attachment_set.md)	 - Replace node type policy attachments

