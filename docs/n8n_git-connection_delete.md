## n8n git-connection delete

Permanently delete a Git connection

### Synopsis

Permanently delete one Git connection together with its local
checkout. Linked projects stay on the instance but lose their remote;
push and pull stop working until a connection is created and cloned
again. This cannot be undone.

To keep the connection and drop only the checkout, use
'n8n git-connection disconnect' instead. Interactive runs ask for
confirmation; non-interactive runs require --yes. Requires
gitConnection:delete.

```
n8n git-connection delete <connection-id> [flags]
```

### Examples

```
  n8n git-connection delete CONNECTION_ID
  n8n git-connection delete CONNECTION_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (JSON is the deletion acknowledgement) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm permanent connection and checkout deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n git-connection](n8n_git-connection.md)	 - Manage the Git connection, its projects, and push/pull sync

