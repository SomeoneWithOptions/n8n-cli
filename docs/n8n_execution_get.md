## n8n execution get

Show one execution

### Synopsis

Show one execution and its run metadata. Add --include-data to include node
input/output, custom data and the workflow definition saved with the run. That
payload may be large; the server omits it when over its display-size limit unless
--ignore-data-size-limit is explicitly given.

Redaction follows the workflow policy by default. Setting
--redact-execution-data=false requests revealed data and needs execution:reveal.
Reading the execution itself requires execution:read.

```
n8n execution get <execution-id> [flags]
```

### Examples

```
  n8n execution get EXECUTION_ID
  n8n execution get EXECUTION_ID --output json
  n8n execution get EXECUTION_ID --include-data --output json
  n8n execution get EXECUTION_ID --include-data --ignore-data-size-limit --redact-execution-data=false --output json
```

### Options

```
      --context string                 saved context to use (default: the current context; see 'n8n config context list')
  -h, --help                           help for get
      --ignore-data-size-limit         return detailed data even when it exceeds the instance display-size limit (default: omit oversized data)
      --include-data                   include detailed node input/output and saved workflow data (default: metadata only)
      --output string                  output format: text or json (JSON includes detailed data only with --include-data) (default "text")
      --redact-execution-data string   detailed-data redaction: true always redacts, false reveals and needs execution:reveal (default: workflow policy)
      --url string                     instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n execution](n8n_execution.md)	 - Inspect, stop, retry and delete workflow executions

