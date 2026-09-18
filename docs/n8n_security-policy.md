## n8n security-policy

Manage instance security policy

### Synopsis

Read or replace the instance security policy for personal-space publishing and
sharing plus the execution-data redaction floor. Start with 'get' to inspect
current values and read-only usage counts, edit that JSON, then pass the complete
document to 'set'.

This licensed Personal Space Policy feature requires securitySettings:manage.
Environment-managed policy can be read but cannot be changed through the API.

```
n8n security-policy [flags]
```

### Examples

```
  n8n security-policy get
  n8n security-policy get --output json > security-policy.json
  n8n security-policy set --input security-policy.json
```

### Options

```
  -h, --help   help for security-policy
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n security-policy get](n8n_security-policy_get.md)	 - Get effective security policy
* [n8n security-policy set](n8n_security-policy_set.md)	 - Fully replace security policy

