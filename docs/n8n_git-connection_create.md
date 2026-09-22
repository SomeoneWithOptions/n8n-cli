## n8n git-connection create

Create the Git connection

### Synopsis

Create the instance's Git connection from strict JSON supplied by
--input. Required fields are name (at most 128 characters),
repositoryUrl, and connectionType (ssh or https). Optional fields are
branchName (at most 255 characters), keyGeneratorType (ed25519 or rsa),
username, and password.

Credentials travel only as --input because secrets are never flags:
process lists and shell history can expose argv. Unknown fields and
trailing JSON documents are refused before transport. Only one
connection can exist; a second answers 409. Requires
gitConnection:create. Follow with 'n8n git-connection clone'.

```
n8n git-connection create [flags]
```

### Examples

```
  n8n git-connection create --input connection.json
  printf '%s\n' '{"name":"prod","repositoryUrl":"git@example.com:org/repo.git","connectionType":"ssh"}' | n8n git-connection create --input -
  n8n git-connection create --input connection.json --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for create
      --input string     Git connection JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the connection returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n git-connection](n8n_git-connection.md)	 - Manage the Git connection, its projects, and push/pull sync

