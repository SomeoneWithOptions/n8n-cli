## n8n promotion project promote

Promote selected project workflows to the remote branch

### Synopsis

Start with 'n8n promotion project changes PROJECT_ID promote' to review IDs.
Push the current state of selected workflows to the project's resolved
promote environment (n8n 2.46 and later) or connection (older): its own,
otherwise the instance one. Prepare it first: 'n8n promotion environment
connect' on 2.46, 'n8n promotion checkout clone' before. Archived
workflows remain archived on the branch; workflows gone from the instance
leave the branch. On 2.46 a workflow that moved into or out of the
project since the branch was written is refused with 400; move it back
first. Unknown deletions reject the whole request. Requires
promotions:promote (gitConnection:push before n8n 2.47), plus
variable:list when workflows use variables.
Commit message is optional; the server supplies a default. Force pushing
is not supported. The result can name a review the Git host opened;
advisory warnings go to stderr.

Pass repeated --workflow-id flags, or use a strict JSON request.
Read request JSON with --input FILE|- (maximum 1 MiB). Input and document flags are
mutually exclusive. Interactive runs ask for confirmation; non-interactive
runs and stdin input require --yes. JSON returns the server result; text
shows counts and the commit after success.

--idempotency-key KEY makes a retry after a timeout or lost response safe:
the same request with the same key returns the first result instead of
running again (auto generates a key and prints it on stderr).

```
n8n promotion project promote <project-id> [flags]
```

### Examples

```
  n8n promotion project promote PROJECT_ID --workflow-id WORKFLOW_ID
  n8n promotion project promote PROJECT_ID --input request.json --yes --output json
```

### Options

```
      --commit-message string     commit message, 1 to 1000 characters (default: server-generated message)
      --context string            saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help                      help for promote
      --idempotency-key string    retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)
      --input string              request JSON file path or - for stdin, maximum 1 MiB (default: use document flags; stdin requires --yes)
      --output string             output format: text or json (default: text; JSON is the full server result) (default "text")
      --url string                instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --workflow-id stringArray   selected workflow ID, 1 to 36 characters; repeat for each ID (default: none; required unless --input is used)
      --yes                       confirm content changes without prompting (default: prompt; required for non-interactive or stdin input)
```

### SEE ALSO

* [n8n promotion project](n8n_promotion_project.md)	 - Manage projects linked to a promotion connection

