## n8n promotion environment create

Create a promotion environment

### Synopsis

Create one promotion environment from strict JSON supplied by --input.
Required fields are name (at most 128 characters), scope (instance or
projects), providerId, remote {"schemaVersion":1,"remoteUrl":"..."},
direction (apply or promote), and settings matching the direction:
apply {"schemaVersion":1,"branchName":"main"} or promote
{"schemaVersion":1,"baseBranchName":"main",
"createBranchOnPromotion":false}. The instance can have one
environment for each direction.

On a gitlab provider the remote URL must belong to the provider: the
environment stores only the repository path and derives remoteUrl from
the provider base URL. Read-only fields from 'environment get --output
json' (id, provider, checkout, createdAt, updatedAt,
remote.repositoryPath) are dropped with a note on stderr; other unknown
fields and trailing JSON documents are refused before transport.
Requires gitConnection:create. Follow with 'n8n promotion environment
connect ENVIRONMENT_ID'.

```
n8n promotion environment create [flags]
```

### Examples

```
  n8n promotion environment create --input environment.json
  printf '%s\n' '{"name":"prod","scope":"instance","providerId":"PROVIDER_ID","remote":{"schemaVersion":1,"remoteUrl":"git@example.com:org/repo.git"},"direction":"apply","settings":{"schemaVersion":1,"branchName":"main"}}' | n8n promotion environment create --input -
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for create
      --input string     promotion environment JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the environment returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)

