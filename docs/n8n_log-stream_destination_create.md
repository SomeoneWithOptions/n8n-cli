## n8n log-stream destination create

Create a log streaming destination

### Synopsis

Create a webhook, syslog, or Sentry destination from a JSON object. The destination
takes effect immediately when enabled. Use 'test ID' afterwards to verify delivery.

Requires the Log Streaming license and eventBusDestination:create.
Credentials, connection URLs, headers, query parameters, certificates and HTTP
options are redacted in all output; unknown response fields are omitted.
Environment-managed writes fail with 409 without changing anything.

Input: type=webhook requires url; type=syslog requires host; type=sentry requires
dsn. Optional common fields: label, enabled, subscribedEvents (names or group
prefixes), anonymizeAuditMessages, circuitBreaker. Syslog protocol is udp/tcp/tls
and facility is 0-23. Webhooks support method, sendHeaders, specifyHeaders,
headerParameters, jsonHeaders, sendQuery, specifyQuery, queryParameters, jsonQuery,
and options. Parameter lists use {"parameters":[{"name":"Authorization","value":"..."}]}.
Additional properties are forwarded as allowed by the API; protect input files
and never pass credentials as flags. [REDACTED] placeholders are rejected.

```
n8n log-stream destination create [flags]
```

### Examples

```
  n8n log-stream destination create --input destination.json
  n8n log-stream destination create --input destination.json --output json
  printf '%s' '{"type":"syslog","host":"syslog.example.com","enabled":false}' | n8n log-stream destination create --input -
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for create
      --input string     destination JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (default text; secrets are always redacted) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n log-stream destination](n8n_log-stream_destination.md)	 - Create, inspect, replace, test, or delete destinations

