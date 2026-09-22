## n8n otel test-trace

Send one test span to an OTLP collector

### Synopsis

Send one test span using collector connection details from strict JSON. This
does not change stored OpenTelemetry settings and needs no confirmation. Required
fields are exporterEndpoint, exporterTracingPath, exporterServiceName,
exporterHeaders, and startupConnectivityTimeoutMs. exporterProtocol is optional
and defaults to http/protobuf.

Environment-managed fields override supplied values before n8n sends the span.
A result with success=false is still a successful API call and is printed for
inspection. Exporter headers may be credentials and are never printed. Requires
otel:manage.

```
n8n otel test-trace [flags]
```

### Examples

```
  n8n otel test-trace --input collector.json
  n8n otel test-trace --input collector.json --output json
  n8n otel test-trace --input - < collector.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for test-trace
      --input string     collector connection JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (request connection details are never printed) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n otel](n8n_otel.md)	 - Manage OpenTelemetry tracing settings

