## n8n log-stream event-type

Inspect streamable event types

### Synopsis

Start with 'list' to find event names for destination subscribedEvents. Group
prefixes such as n8n.workflow subscribe to every event in that group. Requires
the Log Streaming license and eventBusDestination:list.

```
n8n log-stream event-type [flags]
```

### Examples

```
  n8n log-stream event-type list
  n8n log-stream event-type list --output json
```

### Options

```
  -h, --help   help for event-type
```

### SEE ALSO

* [n8n log-stream](n8n_log-stream.md)	 - Manage licensed log streaming destinations
* [n8n log-stream event-type list](n8n_log-stream_event-type_list.md)	 - List event names available for streaming

