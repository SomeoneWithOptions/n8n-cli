## n8n git-connection update

Update a Git connection

### Synopsis

Update one Git connection from strict JSON supplied by --input. Only
the fields present are changed; at least one of name, repositoryUrl,
branchName, connectionType, keyGeneratorType, username, or password is
required.

Run 'n8n git-connection get --output json' first and edit the writable
fields: the response carries read-only id, publicKey, baseCommit, and
timestamps that the update document must not repeat. Credentials travel
only as --input because secrets are never flags. Requires
gitConnection:update.

```
n8n git-connection update <connection-id> [flags]
```

### Examples

```
  n8n git-connection update CONNECTION_ID --input connection-update.json
  printf '%s\n' '{"branchName":"main"}' | n8n git-connection update CONNECTION_ID --input -
  n8n git-connection update CONNECTION_ID --input connection-update.json --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for update
      --input string     Git connection JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the connection returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n git-connection](n8n_git-connection.md)	 - Manage the Git connection, its projects, and push/pull sync

