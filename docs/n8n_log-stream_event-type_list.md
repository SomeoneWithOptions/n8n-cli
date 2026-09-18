## n8n log-stream event-type list

List event names available for streaming

### Synopsis

List all streamable event names without pagination. Use these names or group
prefixes in destination subscribedEvents. Text prints one name per line; JSON
returns a data array. Requires the Log Streaming license and eventBusDestination:list.

```
n8n log-stream event-type list [flags]
```

### Examples

```
  n8n log-stream event-type list
  n8n log-stream event-type list --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for list
      --output string    output format: text or json (default text) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n log-stream event-type](n8n_log-stream_event-type.md)	 - Inspect streamable event types

