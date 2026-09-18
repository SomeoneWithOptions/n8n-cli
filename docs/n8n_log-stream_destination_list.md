## n8n log-stream destination list

List configured log streaming destinations

### Synopsis

List every configured destination without pagination. Text shows a summary table;
JSON returns a data array. Use 'get ID' to inspect one destination.

Requires the Log Streaming license and eventBusDestination:list.
Credentials, connection URLs, headers, query parameters, certificates and HTTP
options are redacted in all output; unknown response fields are omitted.
Environment-managed writes fail with 409 without changing anything.

```
n8n log-stream destination list [flags]
```

### Examples

```
  n8n log-stream destination list
  n8n log-stream destination list --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for list
      --output string    output format: text or json (default text; secrets are always redacted) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n log-stream destination](n8n_log-stream_destination.md)	 - Create, inspect, replace, test, or delete destinations

