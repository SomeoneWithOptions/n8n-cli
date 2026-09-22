## n8n credential create

Create a credential from secret JSON

### Synopsis

Create a credential from one JSON document. Required fields are name, type and
data; optional fields are projectId and isResolvable. Because data contains
secrets, JSON is accepted only through --file or piped --stdin, never argv.

The request body is excluded from debug logs. Output contains metadata only.
Requires credential:create; project creation can have additional role restrictions.

```
n8n credential create [flags]
```

### Examples

```
  n8n credential create --file credential.json
  secret-generator | n8n credential create --stdin --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --file string      read secret-bearing JSON from file (protect and delete the file after use)
  -h, --help             help for create
      --output string    output format: text or json (output never contains credential data) (default "text")
      --stdin            read secret-bearing JSON from piped stdin (refused for an interactive terminal)
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n credential](n8n_credential.md)	 - Manage node credentials without exposing secrets

