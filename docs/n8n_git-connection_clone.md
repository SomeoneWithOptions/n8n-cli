## n8n git-connection clone

Clone the Git repository locally

### Synopsis

Clone the connection's repository into local storage. Omit --branch
to clone the connection's configured branch, or pass it to clone a
different branch of at most 255 characters.

Cloning is safe to repeat and changes no project content by itself:
it only prepares the checkout that 'project add', 'push', and 'pull'
work against, so it needs no confirmation. Clone before pushing or
pulling. Requires gitConnection:clone.

```
n8n git-connection clone <connection-id> [flags]
```

### Examples

```
  n8n git-connection clone CONNECTION_ID
  n8n git-connection clone CONNECTION_ID --branch main
  n8n git-connection clone CONNECTION_ID --branch release --output json
```

### Options

```
      --branch string    branch to clone, at most 255 characters (default: the connection's configured branch)
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for clone
      --output string    output format: text or json (JSON is the connection returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n git-connection](n8n_git-connection.md)	 - Manage the Git connection, its projects, and push/pull sync

