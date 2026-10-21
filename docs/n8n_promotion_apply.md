## n8n promotion apply

Apply the Git remote into this instance

### Synopsis

Check the full package at the apply branch tip and import it into the
instance, overwriting to match. Runs on the instance connection; the
apply checkout must be cloned first.

The server answers one of three statuses, and only one writes:
  applied         content was imported; counts and warnings follow.
  blocked         preflight stopped before any write: missing projects,
                  missing credential or variable bindings, access
                  requirements, or conflicts (including project-deletion
                  and data-table destructive-change). Nothing was applied.
  source-changed  the branch moved since it was reviewed. Nothing was
                  applied.

An applied run rewrites instance content from the remote and cannot be
undone: workflows, folders, credentials, data tables, variables, and
tags are created, updated, or removed to match the branch, and
credential gaps become stubs. Interactive runs ask for confirmation and
non-interactive runs require --yes. Requires gitConnection:pull.

Exit codes: 0 applied, 3 blocked, 4 source-changed, 1 any other error
or an unknown status. --output json prints the full result on stdout
in every status; the next step goes to stderr.

```
n8n promotion apply <connection-id> [flags]
```

### Examples

```
  n8n promotion apply CONNECTION_ID
  n8n promotion apply CONNECTION_ID --yes
  n8n promotion apply CONNECTION_ID --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for apply
      --output string    output format: text or json (JSON is the full apply result: status, git, and counts or preflight) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm overwriting instance content from the remote without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion](n8n_promotion.md)	 - Manage promotion providers, connections, and apply/promote sync

