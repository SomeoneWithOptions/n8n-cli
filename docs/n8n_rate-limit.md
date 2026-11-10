## n8n rate-limit

Manage Public API rate limits

### Synopsis

Read or replace the Public API rate-limit settings: whether limiting is on,
and per-API-key read and write limits across all endpoints (global) and on
one endpoint, each with its own window in seconds. A limit of 0 means no
limit. Requests to these routes count against the limits too.

Fields set by environment variables are listed in envManagedFields: they
report their effective value and cannot be changed through the API.
Requires securitySettings:manage and n8n 2.47 or later.

```
n8n rate-limit [flags]
```

### Examples

```
  n8n rate-limit get
  n8n rate-limit get --output json > rate-limits.json
  n8n rate-limit set --input rate-limits.json --global-write-limit 60
```

### Options

```
  -h, --help   help for rate-limit
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n rate-limit get](n8n_rate-limit_get.md)	 - Get Public API rate-limit settings
* [n8n rate-limit set](n8n_rate-limit_set.md)	 - Fully replace Public API rate-limit settings

