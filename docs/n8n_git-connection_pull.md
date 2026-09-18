## n8n git-connection pull

Pull the Git remote into this instance

### Synopsis

Reset the local clone to the configured branch tip and import
projects into the instance, overwriting to match. The repository must
be cloned first. Preview what is linked with 'n8n git-connection
project list'.

This rewrites instance content from the remote and cannot be undone:
workflows, folders, credentials, data tables, variables, and tags are
created, updated, or removed to match the branch, and credential gaps
become stubs. Interactive runs ask for confirmation and
non-interactive runs require --yes. Requires gitConnection:pull.

```
n8n git-connection pull <connection-id> [flags]
```

### Examples

```
  n8n git-connection pull CONNECTION_ID
  n8n git-connection pull CONNECTION_ID --yes
  n8n git-connection pull CONNECTION_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for pull
      --output string    output format: text or json (JSON is the pull result with counts and commit SHA) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm overwriting instance content from the remote without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n git-connection](n8n_git-connection.md)	 - Manage the Git connection, its projects, and push/pull sync

