## n8n promotion promote

Promote team projects to the Git remote

### Synopsis

Export every team project, commit it, and push to the configured base
branch or a new timestamped branch (the result names it). Personal
projects are ignored. This runs on the instance connection only, so
project links from 'n8n promotion project add' do not select what is
promoted. The promote checkout must be cloned first: run 'n8n promotion
checkout clone CONNECTION_ID promote'. When workflows reference
variables, the API key also needs variable:list.

Pass --commit-message once (1 to 1000 characters). Without --force a
promote the remote would reject is refused with 409 and nothing is
pushed; --force pushes anyway and rewrites remote history. This
mutates the remote branch. Interactive runs ask for confirmation and
non-interactive runs require --yes. Requires gitConnection:push.

--idempotency-key KEY makes a retry after a timeout or lost response safe:
the same request with the same key returns the first result instead of
running again (auto generates a key and prints it on stderr).

```
n8n promotion promote <connection-id> [flags]
```

### Examples

```
  n8n promotion promote CONNECTION_ID --commit-message "sync workflows"
  n8n promotion promote CONNECTION_ID --commit-message "sync" --force --yes
  n8n promotion promote CONNECTION_ID --commit-message "sync" --yes --output json
```

### Options

```
      --commit-message string    commit message for the promote, 1 to 1000 characters (required)
      --context string           saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --force                    promote even when the remote would reject it, rewriting remote history (default: reject a rejected promote with 409)
  -h, --help                     help for promote
      --idempotency-key string   retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)
      --output string            output format: text or json (JSON is the promote result with counts and commit) (default "text")
      --url string               instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes                      confirm promoting every team project to the remote without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion](n8n_promotion.md)	 - Manage promotion providers, connections, and apply/promote sync

