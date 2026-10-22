## n8n credential-policy project

Manage project credential type policies

### Synopsis

Read or replace the project policy. Start with 'get' to obtain scopeId and version; scopeId is null until the first write. Instance scope accepts delegate; project scope accepts allow or deny. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy project [flags]
```

### Examples

```
  n8n credential-policy project get PROJECT_ID
```

### Options

```
  -h, --help   help for project
```

### SEE ALSO

* [n8n credential-policy](n8n_credential-policy.md)	 - Manage credential type availability policies
* [n8n credential-policy project get](n8n_credential-policy_project_get.md)	 - Get the project credential type policy
* [n8n credential-policy project set](n8n_credential-policy_project_set.md)	 - Replace the project credential type policy

