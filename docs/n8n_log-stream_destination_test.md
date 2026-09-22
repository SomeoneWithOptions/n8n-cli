## n8n log-stream destination test

Send one test message to a destination

### Synopsis

Send one test message to the stored destination. This contacts the external
receiver but changes no stored configuration. No input body or confirmation is
needed. Text reports delivery; JSON returns success. success=false is a completed
API call, not a CLI error. Check receiver configuration before testing again.

Requires the Log Streaming license and eventBusDestination:test.
Credentials, connection URLs, headers, query parameters, certificates and HTTP
options are redacted in all output; unknown response fields are omitted.
Environment-managed writes fail with 409 without changing anything.

```
n8n log-stream destination test ID [flags]
```

### Examples

```
  n8n log-stream destination test DESTINATION_ID
  n8n log-stream destination test DESTINATION_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for test
      --output string    output format: text or json (default text; secrets are always redacted) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n log-stream destination](n8n_log-stream_destination.md)	 - Create, inspect, replace, test, or delete destinations

