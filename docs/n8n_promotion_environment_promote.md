## n8n promotion environment promote

Promote team projects to an environment's Git remote

### Synopsis

Export every team project, commit it, and push through an
instance-scope promote environment (n8n 2.46 and later) to its base
branch or a new timestamped branch (the result names it). Before 2.46
use 'n8n promotion promote' with a connection. Start with 'n8n
promotion environment list --direction promote' and connect it first
with 'n8n promotion environment connect'. Personal projects are
ignored, and project links do not select what is promoted; for one
project's workflows use 'n8n promotion project promote'. A workflow on
the branch that moved to a personal project, or to another team project
while its old project still exists, is refused with 400.

--commit-message is optional (1 to 1000 characters; default: a
server-generated message). Without --force a promote the remote would
reject is refused with 409 and nothing is pushed; --force pushes anyway
and rewrites remote history. This mutates the remote branch.
Interactive runs ask for confirmation and non-interactive runs require
--yes. Requires gitConnection:push, plus variable:list when workflows
reference variables.

Text shows the commit, branch, counts, and any review the Git host
opened; JSON is the full result. Advisory warnings go to stderr in both
formats.

--idempotency-key KEY makes a retry after a timeout or lost response safe:
the same request with the same key returns the first result instead of
running again (auto generates a key and prints it on stderr).

```
n8n promotion environment promote <environment-id> [flags]
```

### Examples

```
  n8n promotion environment promote ENVIRONMENT_ID --commit-message "sync workflows"
  n8n promotion environment promote ENVIRONMENT_ID --yes
  n8n promotion environment promote ENVIRONMENT_ID --commit-message "sync" --force --yes --output json
```

### Options

```
      --commit-message string    commit message for the promote, 1 to 1000 characters (default: server-generated message)
      --context string           saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --force                    promote even when the remote would reject it, rewriting remote history (default: reject a rejected promote with 409)
  -h, --help                     help for promote
      --idempotency-key string   retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)
      --output string            output format: text or json (JSON is the promote result with counts, commit, and any review and warnings) (default "text")
      --url string               instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes                      confirm promoting every team project to the remote without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)

