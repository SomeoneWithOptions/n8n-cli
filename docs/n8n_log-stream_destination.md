## n8n log-stream destination

Create, inspect, replace, test, or delete destinations

### Synopsis

Start with 'list', inspect with 'get', then create or fully replace a destination
from a protected JSON file. Use 'test' to send one test message. Update is PUT,
not a patch, and takes effect immediately. Update and delete require confirmation
or --yes. Read output is redacted: restore secrets before a GET-edit-PUT workflow.
All actions require the Log Streaming license and an eventBusDestination scope.

```
n8n log-stream destination [flags]
```

### Examples

```
  n8n log-stream destination list
  n8n log-stream destination get DESTINATION_ID --output json
  n8n log-stream destination update DESTINATION_ID --input destination.json --yes
  n8n log-stream destination test DESTINATION_ID
```

### Options

```
  -h, --help   help for destination
```

### SEE ALSO

* [n8n log-stream](n8n_log-stream.md)	 - Manage licensed log streaming destinations
* [n8n log-stream destination create](n8n_log-stream_destination_create.md)	 - Create a log streaming destination
* [n8n log-stream destination delete](n8n_log-stream_destination_delete.md)	 - Delete a log streaming destination
* [n8n log-stream destination get](n8n_log-stream_destination_get.md)	 - Get a log streaming destination
* [n8n log-stream destination list](n8n_log-stream_destination_list.md)	 - List configured log streaming destinations
* [n8n log-stream destination test](n8n_log-stream_destination_test.md)	 - Send one test message to a destination
* [n8n log-stream destination update](n8n_log-stream_destination_update.md)	 - Fully replace a log streaming destination

