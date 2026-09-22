## n8n otel get

Get effective OpenTelemetry settings

### Synopsis

Get every effective OpenTelemetry setting exposed by n8n. Omitted protocol
values are reported as the server default, http/protobuf. Use JSON as the
starting point for a GET-edit-PUT workflow.

Text output never prints exporter headers. JSON output includes them because a
full replacement requires exporterHeaders; protect redirected files as secrets.
Requires otel:manage.

```
n8n otel get [flags]
```

### Examples

```
  n8n otel get
  n8n otel get --output json
  umask 077 && n8n otel get --output json > otel.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON includes possibly sensitive exporter headers) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n otel](n8n_otel.md)	 - Manage OpenTelemetry tracing settings

