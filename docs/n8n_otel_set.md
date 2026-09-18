## n8n otel set

Fully replace OpenTelemetry settings

### Synopsis

Fully replace the instance OpenTelemetry configuration from strict JSON. This
is a PUT, not a patch: every field is required except exporterProtocol. Omitting
that field selects http/protobuf. Start from 'n8n otel get --output json'.

A successful update is applied to the running instance immediately and can
change or stop exported traces, so interactive runs ask for confirmation and
non-interactive runs require --yes. --input - also requires --yes because stdin
cannot hold both JSON and a confirmation answer.

Environment-managed fields are read-only. Re-submit their effective GET values
unchanged, or change the environment configuration. Exporter headers may be
credentials; response details are redacted on errors. Requires otel:manage.

```
n8n otel set [flags]
```

### Examples

```
  umask 077 && n8n otel get --output json > otel.json
  n8n otel set --input otel.json
  n8n otel set --input otel.json --yes --output json
  n8n otel set --input - --yes < otel.json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for set
      --input string     full OpenTelemetry settings JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON includes possibly sensitive exporter headers) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm full replacement and immediate runtime application without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n otel](n8n_otel.md)	 - Manage OpenTelemetry tracing settings

