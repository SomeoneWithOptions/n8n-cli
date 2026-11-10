## n8n rate-limit set

Fully replace Public API rate-limit settings

### Synopsis

Fully replace the Public API rate-limit settings. This is a PUT, not a patch:
enabled and all eight limit and window fields are required. Take them from
--input (start from 'n8n rate-limit get --output json'), from flags, or both;
a flag overrides the --input value. Limits are request counts per API key,
at least 0 (0 for no limit); windows are seconds, at least 1. When limiting
is enabled, at least one limit must be above 0.

envManagedFields in --input is read-only: it is dropped before sending and
reported on stderr. Fields set by environment variables can be repeated with
their current value but not changed; a different value answers 409 and
nothing changes.

New limits apply at once to every API key, including the one this CLI uses,
so interactive runs ask for confirmation and non-interactive runs require
--yes. --input - also requires --yes because stdin cannot hold both JSON and
a confirmation answer. Requires securitySettings:manage and n8n 2.47 or later.

```
n8n rate-limit set [flags]
```

### Examples

```
  n8n rate-limit get --output json > rate-limits.json
  n8n rate-limit set --input rate-limits.json --global-write-limit 60
  n8n rate-limit set --input - --yes < rate-limits.json
  n8n rate-limit set --enabled=false --global-read-limit 0 --global-read-window-seconds 60 \
      --global-write-limit 0 --global-write-window-seconds 60 --endpoint-read-limit 0 \
      --endpoint-read-window-seconds 60 --endpoint-write-limit 0 --endpoint-write-window-seconds 60 --yes
```

### Options

```
      --context string                      saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --enabled                             turn Public API rate limiting on or off; true or false, overrides enabled in --input (default: the --input value)
      --endpoint-read-limit int             requests allowed per API key in the window, 0 for no limit; overrides endpointReadLimit in --input (default: the --input value)
      --endpoint-read-window-seconds int    window in seconds, at least 1; overrides endpointReadWindowSeconds in --input (default: the --input value)
      --endpoint-write-limit int            requests allowed per API key in the window, 0 for no limit; overrides endpointWriteLimit in --input (default: the --input value)
      --endpoint-write-window-seconds int   window in seconds, at least 1; overrides endpointWriteWindowSeconds in --input (default: the --input value)
      --global-read-limit int               requests allowed per API key in the window, 0 for no limit; overrides globalReadLimit in --input (default: the --input value)
      --global-read-window-seconds int      window in seconds, at least 1; overrides globalReadWindowSeconds in --input (default: the --input value)
      --global-write-limit int              requests allowed per API key in the window, 0 for no limit; overrides globalWriteLimit in --input (default: the --input value)
      --global-write-window-seconds int     window in seconds, at least 1; overrides globalWriteWindowSeconds in --input (default: the --input value)
  -h, --help                                help for set
      --input string                        full rate-limit settings JSON file path, or - for stdin, e.g. from 'n8n rate-limit get --output json' (default: none; flags supply every field; maximum 1 MiB)
      --output string                       output format: text or json (JSON is the effective settings returned after replacement) (default "text")
      --url string                          instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes                                 confirm replacing the limits, which apply at once to every API key, without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n rate-limit](n8n_rate-limit.md)	 - Manage Public API rate limits

