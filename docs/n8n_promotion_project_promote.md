## n8n promotion project promote

Promote selected project workflows to the remote branch

### Synopsis

Start with 'n8n promotion project changes PROJECT_ID promote' to review IDs.
Push the current state of selected workflows to the project's resolved
promotion connection (its own, otherwise the instance connection). Clone
the promote direction first. Archived workflows remain archived on the
branch; workflows no longer owned by the project leave the branch. Unknown
deletions reject the whole request. Requires gitConnection:push, plus
variable:list when workflows use variables. Commit message is optional;
the server supplies a default. Force pushing is not supported.

Pass repeated --workflow-id flags, or use a strict JSON request.
Read request JSON with --input FILE|- (maximum 1 MiB). Input and document flags are
mutually exclusive. Interactive runs ask for confirmation; non-interactive
runs and stdin input require --yes. JSON returns the server result; text
shows counts and the commit after success.

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
      --input string              request JSON file path or - for stdin, maximum 1 MiB (default: use document flags; stdin requires --yes)
      --output string             output format: text or json (default: text; JSON is the full server result) (default "text")
      --url string                instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --workflow-id stringArray   selected workflow ID, 1 to 36 characters; repeat for each ID (default: none; required unless --input is used)
      --yes                       confirm content changes without prompting (default: prompt; required for non-interactive or stdin input)
```

### SEE ALSO

* [n8n promotion project](n8n_promotion_project.md)	 - Manage projects linked to a promotion connection

