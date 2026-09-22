## n8n credential update

Update a credential from secret JSON

### Synopsis

Update an owned credential from one JSON document. Accepted fields are name, type,
data, isGlobal, isResolvable and isPartialData. Set isPartialData=true to merge
provided data with stored data; false replaces the full data object. Changing type
requires data. Managed credentials cannot be edited.

Secret JSON is accepted only through --file or piped --stdin and is excluded from
debug logs and output. Requires credential:update and ownership.

```
n8n credential update <credential-id> [flags]
```

### Examples

```
  n8n credential update CREDENTIAL_ID --file update.json
  secret-generator | n8n credential update CREDENTIAL_ID --stdin --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --file string      read secret-bearing JSON from file (protect and delete the file after use)
  -h, --help             help for update
      --output string    output format: text or json (output never contains credential data) (default "text")
      --stdin            read secret-bearing JSON from piped stdin (refused for an interactive terminal)
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n credential](n8n_credential.md)	 - Manage node credentials without exposing secrets

