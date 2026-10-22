## n8n credential-policy instance

Manage instance credential type policies

### Synopsis

Read or replace the instance policy. Start with 'get' to obtain scopeId and version; scopeId is null until the first write. Instance scope accepts delegate; project scope accepts allow or deny. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy instance [flags]
```

### Examples

```
  n8n credential-policy instance get
```

### Options

```
  -h, --help   help for instance
```

### SEE ALSO

* [n8n credential-policy](n8n_credential-policy.md)	 - Manage credential type availability policies
* [n8n credential-policy instance get](n8n_credential-policy_instance_get.md)	 - Get the instance credential type policy
* [n8n credential-policy instance set](n8n_credential-policy_instance_set.md)	 - Replace the instance credential type policy

