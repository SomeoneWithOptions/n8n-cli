## n8n otel

Manage OpenTelemetry tracing settings

### Synopsis

Read, fully replace, and test the instance OpenTelemetry tracing configuration.
Start with 'get --output json', edit the complete document, then use 'set'.
Exporter headers may contain collector credentials: text output only reports
whether they are configured, while JSON includes them for the edit workflow.

Set applies a successful replacement to the running instance immediately and
therefore requires confirmation. Test-trace sends one span without changing
stored settings. Every action requires otel:manage.

```
n8n otel [flags]
```

### Examples

```
  n8n otel get
  n8n otel get --output json > otel.json
  n8n otel set --input otel.json
  n8n otel test-trace --input collector.json
```

### Options

```
  -h, --help   help for otel
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n otel get](n8n_otel_get.md)	 - Get effective OpenTelemetry settings
* [n8n otel set](n8n_otel_set.md)	 - Fully replace OpenTelemetry settings
* [n8n otel test-trace](n8n_otel_test-trace.md)	 - Send one test span to an OTLP collector

