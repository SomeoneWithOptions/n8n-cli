## n8n promotion project apply-continue

Continue a reviewed project apply after binding setup

### Synopsis

Start with 'n8n promotion project changes PROJECT_ID apply' to review workflow IDs.
The project's own apply environment (n8n 2.46 and later) or connection
(older) resolves first, otherwise the instance one. It must be prepared:
'n8n promotion environment connect' on 2.46, 'n8n promotion checkout
clone' before. Selected IDs on the branch are imported; selected workflows
owned by this project whose IDs are absent from the branch are removed.
On 2.46 a selected workflow also moves within the project to match the
branch, with the folders that hold it, which take the branch name; one
the branch and the instance hold in different projects is refused with
409 before any write. Unselected content stays unchanged. Duplicate or
invalid IDs reject the whole request. Requires promotions:apply
(gitConnection:pull before n8n 2.47).
Use --expected-from FILE|- with changes JSON or a blocked apply result,
or supply --expected-environment-id (n8n 2.46 and later) or
--expected-config-id (older; mutually exclusive), --expected-branch, and
--expected-commit together (full lowercase 40- or 64-digit hex SHA). A
null commit cannot be used. --expected-from passes through the
identifier the instance returned: environmentId (2.46) or configId (older).
Expected source is required; use the blocked result's environmentId (2.46) or configId (older)
and git. The server keeps no session and provides no exactly-once guarantee.
Resend the same full workflow ID selection used for the initial apply.
--confirm-destructive-changes permits removal or retyping of data-table
columns and loss of their values; rows and other columns remain.
This covers every such change at continue time, including changes absent
from the reviewed result.
--expected-preview-revision (n8n 2.47 and later) pins the reviewed Apply
preview; when this instance changed since, the server answers
preview-changed and writes nothing. Older instances reject it.
Exit codes: 0 applied, 3 blocked, 4 source-changed or preview-changed
(n8n 2.47 and later), 1 other error. Only applied writes content.
Resolve blocked items before apply-continue; review source-changed and
preview-changed results again. JSON prints the full status result; text
shows the blocking check (preflight before n8n 2.47) or counts and
warnings. Hints go to stderr.

Pass repeated --workflow-id flags, or use a strict JSON request.
Read request JSON with --input FILE|- (maximum 1 MiB). Input and document flags are
mutually exclusive. Interactive runs ask for confirmation; non-interactive
runs and stdin input require --yes. JSON returns the server result; text
shows counts and the commit after success.

--idempotency-key KEY makes a retry after a timeout or lost response safe:
the same request with the same key returns the first result instead of
running again (auto generates a key and prints it on stderr).

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
      --confirm-destructive-changes        allow all destructive changes at continue time, including unreviewed column value loss and, for full apply, team-project deletion (default: false)
      --context string                     saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --expected-branch string             reviewed branch name (default: omitted; requires the expected ID and commit)
      --expected-commit string             reviewed lowercase full 40- or 64-digit hex commit SHA (default: omitted; requires the expected ID and branch)
      --expected-config-id string          reviewed config ID on n8n 2.45 and earlier, 1 to 36 characters (default: omitted; requires expected branch and commit; excludes --expected-environment-id)
      --expected-environment-id string     reviewed environment ID on n8n 2.46 and later, 1 to 36 characters (default: omitted; requires expected branch and commit; excludes --expected-config-id)
      --expected-from string               changes or blocked-apply JSON file, or - for stdin (default: explicit expected-source flags; stdin requires --yes)
      --expected-preview-revision string   revision of the reviewed Apply preview, 64 lowercase hex digits; a changed preview answers preview-changed and writes nothing (n8n 2.47 and later; default: omitted)
  -h, --help                               help for apply-continue
      --idempotency-key string             retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)
      --input string                       request JSON file path or - for stdin, maximum 1 MiB (default: use document flags; stdin requires --yes)
      --output string                      output format: text or json (default: text; JSON is the full server result) (default "text")
      --url string                         instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --workflow-id stringArray            selected workflow ID, 1 to 36 characters; repeat for each ID (default: none; required unless --input is used)
      --yes                                confirm content changes without prompting (default: prompt; required for non-interactive or stdin input)
```

### SEE ALSO

* [n8n promotion project](n8n_promotion_project.md)	 - Manage projects linked to a promotion connection

