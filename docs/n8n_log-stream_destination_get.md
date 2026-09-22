## n8n log-stream destination get

Get a log streaming destination

### Synopsis

Get a destination by ID. Text shows a summary; JSON shows known configuration
fields with secrets redacted. For GET-edit-PUT, restore redacted fields from a
protected source and review the complete document before 'update'.

Requires the Log Streaming license and eventBusDestination:read.
Credentials, connection URLs, headers, query parameters, certificates and HTTP
options are redacted in all output; unknown response fields are omitted.
Environment-managed writes fail with 409 without changing anything.

```
n8n log-stream destination get ID [flags]
```

### Examples

```
  n8n log-stream destination get DESTINATION_ID
  n8n log-stream destination get DESTINATION_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (default text; secrets are always redacted) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n log-stream destination](n8n_log-stream_destination.md)	 - Create, inspect, replace, test, or delete destinations

