## n8n log-stream destination update

Fully replace a log streaming destination

### Synopsis

Fully replace a destination by ID using PUT, not a patch. Omitted optional fields
may reset to server defaults. Start from 'get ID --output json', restore redacted
secrets from a protected source, then edit the complete document. The read-only id
is stripped from input. Replacement applies immediately and may stop delivery.
Interactive runs ask for confirmation; non-interactive runs require --yes.
--input - requires --yes because stdin cannot also answer confirmation.

Requires the Log Streaming license and eventBusDestination:update.
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
n8n log-stream destination update ID [flags]
```

### Examples

```
  n8n log-stream destination update DESTINATION_ID --input destination.json
  n8n log-stream destination update DESTINATION_ID --input destination.json --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for update
      --input string     destination JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (default text; secrets are always redacted) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm replacement or deletion without prompting (default false; required when non-interactive)
```

### SEE ALSO

* [n8n log-stream destination](n8n_log-stream_destination.md)	 - Create, inspect, replace, test, or delete destinations

