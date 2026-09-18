## n8n git-connection disconnect

Remove the local Git checkout

### Synopsis

Remove the local clone of one Git connection. The connection and its
authentication material are retained, and linked projects stay linked,
but push and pull stop working until 'clone' runs again.

To remove the connection itself as well, use 'n8n git-connection
delete' instead. Interactive runs ask for confirmation;
non-interactive runs require --yes. Requires gitConnection:clone.

```
n8n git-connection disconnect <connection-id> [flags]
```

### Examples

```
  n8n git-connection disconnect CONNECTION_ID
  n8n git-connection disconnect CONNECTION_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for disconnect
      --output string    output format: text or json (JSON is the connection returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm checkout removal without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n git-connection](n8n_git-connection.md)	 - Manage the Git connection, its projects, and push/pull sync

