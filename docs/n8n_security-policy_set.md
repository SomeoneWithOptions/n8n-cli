## n8n security-policy set

Fully replace security policy

### Synopsis

Fully replace every writable field in the instance security policy from strict
JSON supplied by --input. This is a PUT, not a patch: personalSpacePublishing,
personalSpaceSharing, and redactionEnforcement.floor are all required, even when
unchanged. floor must be off, production, or all.

Start from 'n8n security-policy get --output json'. Read-only usage counts in that
document are accepted and ignored by the server. Requires securitySettings:manage
and the licensed Personal Space Policy feature. Environment-managed policy is
rejected with 409 and no changes are made.

```
n8n security-policy set [flags]
```

### Examples

```
  n8n security-policy get --output json > security-policy.json
  n8n security-policy set --input security-policy.json
  n8n security-policy set --input - < security-policy.json
  n8n security-policy set --input security-policy.json --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for set
      --input string     full security policy JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the effective policy returned after replacement) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n security-policy](n8n_security-policy.md)	 - Manage instance security policy

