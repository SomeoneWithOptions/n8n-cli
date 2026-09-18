## n8n log-stream destination delete

Delete a log streaming destination

### Synopsis

Permanently remove this destination and stop future event delivery to it. Events
already delivered to the external receiver are not removed. Interactive runs ask
for confirmation; non-interactive runs require --yes. Use 'list' to verify removal.

Requires the Log Streaming license and eventBusDestination:delete.
Credentials, connection URLs, headers, query parameters, certificates and HTTP
options are redacted in all output; unknown response fields are omitted.
Environment-managed writes fail with 409 without changing anything.

```
n8n log-stream destination delete ID [flags]
```

### Examples

```
  n8n log-stream destination delete DESTINATION_ID
  n8n log-stream destination delete DESTINATION_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (default text; secrets are always redacted) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm replacement or deletion without prompting (default false; required when non-interactive)
```

### SEE ALSO

* [n8n log-stream destination](n8n_log-stream_destination.md)	 - Create, inspect, replace, test, or delete destinations

