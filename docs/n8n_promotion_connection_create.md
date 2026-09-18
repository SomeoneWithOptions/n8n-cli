## n8n promotion connection create

Create a promotion connection

### Synopsis

Create one promotion connection from strict JSON supplied by --input.
Required fields are name (at most 128 characters), scope (instance or
projects), providerId, and target {"schemaVersion":1,
"remoteUrl":"..."}. An optional configs object carries the apply
config {"settings":{"schemaVersion":1,"branchName":"main"}} and
the promote config {"settings":{"schemaVersion":1,
"baseBranchName":"main","createBranchOnPromotion":false}}.

Unknown fields and trailing JSON documents are refused before
transport. Requires gitConnection:create. Follow with
'n8n promotion config apply set' and 'n8n promotion config promote
set' when the configs were omitted.

```
n8n promotion connection create [flags]
```

### Examples

```
  n8n promotion connection create --input connection.json
  n8n promotion connection create --input connection.json --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for create
      --input string     promotion connection JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the connection returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n promotion connection](n8n_promotion_connection.md)	 - Manage promotion connections

