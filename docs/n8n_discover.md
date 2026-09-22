## n8n discover

List the API capabilities available to the current credential

### Synopsis

List the API capabilities available to the current credential.

Use this to find out what this instance and this credential can actually do
before running a resource command, and to explain a 403: the response holds
the credential's scopes plus every resource, operation and endpoint it may
reach. A resource missing from the output means it is unavailable to you,
which covers an unlicensed feature and an unscoped credential alike.

Narrow the output with --resource and --operation. Add --schemas to inline
each endpoint's request body schema, which is enough to build a request
without fetching the full OpenAPI document.

Text output is a summary and a resource table; --output json emits scopes,
endpoints and schemas in full and is the stable form for scripts and agents.
This is a read-only diagnostic: it changes nothing on the instance.

Prerequisite: 'n8n auth login'. Next step: run the resource command you
found here.

```
n8n discover [flags]
```

### Examples

```
  n8n discover
  n8n discover --resource workflow
  n8n discover --resource workflow --operation create --schemas --output json
  n8n discover --output json | jq -r '.scopes[]'
  n8n discover --context production
```

### Options

```
      --context string     saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help               help for discover
      --operation string   operation to filter by, e.g. read, list, create (default: all operations)
      --output string      output format: text or json (default text; json is stable for scripting) (default "text")
      --resource string    resource key to filter by, e.g. workflow, credential, datatable (default: all resources)
      --schemas            include each endpoint's request body schema (visible with --output json)
      --url string         instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API

