## n8n promotion environment update

Update a promotion environment

### Synopsis

Update one promotion environment from strict JSON supplied by --input.
Only the fields present are changed; at least one of name, providerId,
remote, or settings is required. A remote replaces the whole saved
remote and settings replace the whole saved settings, which must match
the environment's direction (promote always needs
createBranchOnPromotion). Scope and direction cannot change.

Start from 'n8n promotion environment get ID --output json': read-only
fields (id, scope, direction, provider, checkout, createdAt, updatedAt,
remote.repositoryPath) are dropped with a note on stderr, and a
direction in the document is used to check the settings first. A 409
means the provider changed during the update: reload and retry.

A new remote or branch can leave the local checkout stale, so
interactive runs ask for confirmation; non-interactive runs and
--input - require --yes. Requires gitConnection:update.

```
n8n promotion environment update <environment-id> [flags]
```

### Examples

```
  n8n promotion environment update ENVIRONMENT_ID --input environment-update.json
  printf '%s\n' '{"name":"staging"}' | n8n promotion environment update ENVIRONMENT_ID --input - --yes
  n8n promotion environment update ENVIRONMENT_ID --input environment.json --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for update
      --input string     promotion environment update JSON file path, or - for stdin (required; maximum 1 MiB; - requires --yes)
      --output string    output format: text or json (JSON is the environment returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm the environment update without prompting (required when stdin is not interactive or with --input -)
```

### SEE ALSO

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)

