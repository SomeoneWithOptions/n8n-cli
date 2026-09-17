## n8n audit

Generate security audits of an n8n instance

### Synopsis

Generate security audits of an n8n instance.

There is one action today: 'n8n audit generate', which asks the instance for
a risk report covering credentials, database, nodes, filesystem and instance
settings. Auditing is read-only, so it is safe to run against production.

Typical workflow: 'n8n auth login', then 'n8n audit generate' to see the
findings, then 'n8n audit generate --output json' to feed them to a script.

```
n8n audit [flags]
```

### Examples

```
  n8n audit generate
  n8n audit generate --category credentials --output json
```

### Options

```
  -h, --help   help for audit
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n audit generate](n8n_audit_generate.md)	 - Generate a security audit of the instance

