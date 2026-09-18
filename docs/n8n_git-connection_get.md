## n8n git-connection get

Show one Git connection

### Synopsis

Show one Git connection by ID. The response is the public connection
document: name, repository URL, branch, connection type, the SSH public
key to deploy, the key generator, the base commit, and timestamps.
Secrets are never returned. Find the ID with 'n8n git-connection
list'. Requires gitConnection:read.

```
n8n git-connection get <connection-id> [flags]
```

### Examples

```
  n8n git-connection get CONNECTION_ID
  n8n git-connection get CONNECTION_ID --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the connection object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n git-connection](n8n_git-connection.md)	 - Manage the Git connection, its projects, and push/pull sync

