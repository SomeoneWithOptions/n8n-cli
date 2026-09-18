## n8n log-stream

Manage licensed log streaming destinations

### Synopsis

Stream n8n events to webhook, syslog, or Sentry destinations. Requires the Log
Streaming license and the action's eventBusDestination scope. Start with
'event-type list', then 'destination list', create a destination, and test it.

Destination credentials, URLs, headers, query parameters, certificates and HTTP
options are redacted in both text and JSON. Keep original configuration in a
protected file; never submit redacted output unchanged. Environment-managed
destinations can be read but not created, updated, or deleted.

```
n8n log-stream [flags]
```

### Examples

```
  n8n log-stream event-type list
  n8n log-stream destination list
  n8n log-stream destination create --input destination.json
  n8n log-stream destination test DESTINATION_ID
```

### Options

```
  -h, --help   help for log-stream
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n log-stream destination](n8n_log-stream_destination.md)	 - Create, inspect, replace, test, or delete destinations
* [n8n log-stream event-type](n8n_log-stream_event-type.md)	 - Inspect streamable event types

