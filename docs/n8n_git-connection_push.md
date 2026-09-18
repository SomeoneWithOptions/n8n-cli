## n8n git-connection push

Push all team projects to the Git remote

### Synopsis

Export all team projects, commit them, and push to the connection's
configured branch. Personal projects are ignored. The repository must
be cloned first: run 'n8n git-connection clone' and link projects
with 'n8n git-connection project add'.

Pass --commit-message once (1 to 1000 characters). Without --force a
push the remote would reject is refused with 409 and nothing is
pushed; --force pushes anyway and rewrites remote history. This
mutates the remote branch. Interactive runs ask for confirmation and
non-interactive runs require --yes. Requires gitConnection:push.

```
n8n git-connection push <connection-id> [flags]
```

### Examples

```
  n8n git-connection push CONNECTION_ID --commit-message "sync workflows"
  n8n git-connection push CONNECTION_ID --commit-message "sync" --force --yes
  n8n git-connection push CONNECTION_ID --commit-message "sync" --yes --output json
```

### Options

```
      --commit-message string   commit message for the push, 1 to 1000 characters (required)
      --context string          saved context to use (default: the current context; see 'n8n config context list')
      --force                   push even when the remote would reject it, rewriting remote history (default: reject a rejected push with 409)
  -h, --help                    help for push
      --output string           output format: text or json (JSON is the push result with counts and commit SHA) (default "text")
      --url string              instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes                     confirm pushing every team project to the remote without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n git-connection](n8n_git-connection.md)	 - Manage the Git connection, its projects, and push/pull sync

