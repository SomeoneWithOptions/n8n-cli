## n8n otel set

Fully replace OpenTelemetry settings

### Synopsis

Fully replace the instance OpenTelemetry configuration from strict JSON. This
is a PUT, not a patch: every field is required except exporterProtocol,
emitWorkflowStartSpan, and emitNodeStartSpan. Omitting exporterProtocol selects
http/protobuf. The two start-span fields are omitted = false on the server: a
document without them turns both spans off, so pass the current values or use
--input from 'n8n otel get --output json', which round-trips them losslessly.
--emit-workflow-start-span and --emit-node-start-span set or override them in
the --input document. Instances older than these settings do not report them;
leave them out there.

A successful update is applied to the running instance immediately and can
change or stop exported traces, so interactive runs ask for confirmation and
non-interactive runs require --yes. --input - also requires --yes because stdin
cannot hold both JSON and a confirmation answer.

Environment-managed fields are read-only. Re-submit their effective GET values
unchanged, or change the environment configuration. Omitting a start-span field
that the environment pins to true is a change and answers 409. Exporter headers
may be credentials; response details are redacted on errors. Requires
otel:manage.

```
n8n otel set [flags]
```

### Examples

```
  umask 077 && n8n otel get --output json > otel.json
  n8n otel set --input otel.json
  n8n otel set --input otel.json --yes --output json
  n8n otel set --input otel.json --emit-workflow-start-span --emit-node-start-span=false
  n8n otel set --input - --yes < otel.json
```

### Options

```
      --context string             saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --emit-node-start-span       emit a 0 ms node.execute.started span when each node span starts, doubling node spans; true or false, overrides emitNodeStartSpan in --input (default: the --input value, and false on the server when both omit it)
      --emit-workflow-start-span   emit a 0 ms workflow.execute.started span when each execution starts; true or false, overrides emitWorkflowStartSpan in --input (default: the --input value, and false on the server when both omit it)
  -h, --help                       help for set
      --input string               full OpenTelemetry settings JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string              output format: text or json (JSON includes possibly sensitive exporter headers) (default "text")
      --url string                 instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes                        confirm full replacement and immediate runtime application without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n otel](n8n_otel.md)	 - Manage OpenTelemetry tracing settings

