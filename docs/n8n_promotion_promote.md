## n8n promotion promote

Promote team projects to the Git remote

### Synopsis

Export all team projects, commit them, and push to the promote
checkout's branch. The promote checkout must be cloned first: run 'n8n
promotion checkout clone CONNECTION_ID promote' and link projects
with 'n8n promotion project add'.

Pass --commit-message once (1 to 1000 characters). Without --force a
promote the remote would reject is refused with 409 and nothing is
pushed; --force pushes anyway and rewrites remote history. This
mutates the remote branch. Interactive runs ask for confirmation and
non-interactive runs require --yes. Requires gitConnection:push.

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
      --commit-message string   commit message for the promote, 1 to 1000 characters (required)
      --context string          saved context to use (default: the current context; see 'n8n config context list')
      --force                   promote even when the remote would reject it, rewriting remote history (default: reject a rejected promote with 409)
  -h, --help                    help for promote
      --output string           output format: text or json (JSON is the promote result with counts and commit) (default "text")
      --url string              instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes                     confirm promoting every team project to the remote without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion](n8n_promotion.md)	 - Manage promotion providers, connections, and apply/promote sync

