## n8n promotion project apply-continue

Continue a reviewed project apply after binding setup

### Synopsis

Start with 'n8n promotion project changes PROJECT_ID apply' to review workflow IDs.
The project's own connection resolves first, otherwise the instance connection.
The apply direction must be cloned. Selected IDs on the branch are imported;
selected workflows owned by this project whose IDs are absent from the branch
are removed. Unselected content stays unchanged. Duplicate or invalid IDs
reject the whole request. Requires gitConnection:pull.
Use --expected-from FILE|- with changes JSON or a blocked apply result,
or supply all three --expected-config-id, --expected-branch, --expected-commit
flags (full lowercase 40- or 64-digit hex SHA). A null commit cannot be used.
Expected source is required; use the blocked result's configId and git.
The server keeps no session and provides no exactly-once guarantee.
Resend the same full workflow ID selection used for the initial apply.
--confirm-destructive-changes permits removal or retyping of data-table
columns and loss of their values; rows and other columns remain.
This covers every such change at continue time, including changes absent
from the reviewed result.
Exit codes: 0 applied, 3 blocked, 4 source-changed, 1 other error. Only
applied writes content. Resolve blocked bindings before apply-continue;
review source-changed results again. JSON prints the full status result;
text shows preflight details or counts and warnings. Hints go to stderr.

Pass repeated --workflow-id flags, or use a strict JSON request.
Read request JSON with --input FILE|- (maximum 1 MiB). Input and document flags are
mutually exclusive. Interactive runs ask for confirmation; non-interactive
runs and stdin input require --yes. JSON returns the server result; text
shows counts and the commit after success.

```
n8n promotion project apply-continue <project-id> [flags]
```

### Examples

```
  n8n promotion project apply-continue PROJECT_ID --workflow-id WORKFLOW_ID --expected-from blocked.json
  n8n promotion project apply-continue PROJECT_ID --input request.json --yes --output json
```

### Options

```
      --confirm-destructive-changes   allow all destructive changes at continue time, including unreviewed column value loss and, for full apply, team-project deletion (default: false)
      --context string                saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --expected-branch string        reviewed branch name (default: omitted; requires expected config ID and commit)
      --expected-commit string        reviewed lowercase full 40- or 64-digit hex commit SHA (default: omitted; requires expected config ID and branch)
      --expected-config-id string     reviewed config ID, 1 to 36 characters (default: omitted; requires expected branch and commit)
      --expected-from string          changes or blocked-apply JSON file, or - for stdin (default: explicit expected-source flags; stdin requires --yes)
  -h, --help                          help for apply-continue
      --input string                  request JSON file path or - for stdin, maximum 1 MiB (default: use document flags; stdin requires --yes)
      --output string                 output format: text or json (default: text; JSON is the full server result) (default "text")
      --url string                    instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --workflow-id stringArray       selected workflow ID, 1 to 36 characters; repeat for each ID (default: none; required unless --input is used)
      --yes                           confirm content changes without prompting (default: prompt; required for non-interactive or stdin input)
```

### SEE ALSO

* [n8n promotion project](n8n_promotion_project.md)	 - Manage projects linked to a promotion connection

