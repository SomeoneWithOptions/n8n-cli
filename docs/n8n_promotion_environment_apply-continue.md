## n8n promotion environment apply-continue

Continue a reviewed environment apply after binding setup

### Synopsis

Continue a full environment apply (n8n 2.46 and later) after resolving its
check items. Start with 'n8n promotion environment apply ENVIRONMENT_ID
--output json'. The instance-scope apply environment must still be
connected. Every team project is synchronized: workflows, folders,
credentials, tables, variables, and tags can be created, updated, moved,
renamed, or removed to match. Requires promotions:apply (gitConnection:pull
before n8n 2.47) and the same permissions as the initial apply.

Use --expected-from FILE|- with changes JSON or a blocked apply result,
or supply --expected-environment-id, --expected-branch, and
--expected-commit together (full lowercase 40- or 64-digit hex SHA). A
null commit cannot be used. --expected-from passes through the
identifier the instance returned: environmentId.
Expected source is required; use the blocked result's environmentId
and git. The server keeps no session and provides no exactly-once guarantee.
--confirm-destructive-changes permits removal or retyping of data-table
columns and loss of their values; rows and other columns remain.
It also permits deleting team projects with everything in them.
This covers every such change at continue time, including changes absent
from the reviewed result.
Exit codes: 0 applied, 3 blocked, 4 source-changed or preview-changed
(n8n 2.47 and later), 1 other error. Only applied writes content.
Resolve blocked items before apply-continue; review source-changed and
preview-changed results again. JSON prints the full status result; text
shows the blocking check (preflight before n8n 2.47) or counts and
warnings. Hints go to stderr.
Read request JSON with --input FILE|- (maximum 1 MiB). Input and document flags are
mutually exclusive. Interactive runs ask for confirmation; non-interactive
runs and stdin input require --yes. JSON returns the server result; text
shows counts and the commit after success.

--idempotency-key KEY makes a retry after a timeout or lost response safe:
the same request with the same key returns the first result instead of
running again (auto generates a key and prints it on stderr).

```
n8n promotion environment apply-continue <environment-id> [flags]
```

### Examples

```
  n8n promotion environment apply-continue ENVIRONMENT_ID --expected-from blocked.json
  n8n promotion environment apply-continue ENVIRONMENT_ID --input request.json --yes --output json
```

### Options

```
      --confirm-destructive-changes      allow all destructive changes at continue time, including unreviewed column value loss and, for full apply, team-project deletion (default: false)
      --context string                   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --expected-branch string           reviewed branch name (default: omitted; requires the expected ID and commit)
      --expected-commit string           reviewed lowercase full 40- or 64-digit hex commit SHA (default: omitted; requires the expected ID and branch)
      --expected-environment-id string   reviewed environment ID, 1 to 36 characters (default: omitted; requires expected branch and commit)
      --expected-from string             changes or blocked-apply JSON file, or - for stdin (default: explicit expected-source flags; stdin requires --yes)
  -h, --help                             help for apply-continue
      --idempotency-key string           retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)
      --input string                     request JSON file path or - for stdin, maximum 1 MiB (default: use document flags; stdin requires --yes)
      --output string                    output format: text or json (default: text; JSON is the full server result) (default "text")
      --url string                       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes                              confirm content changes without prompting (default: prompt; required for non-interactive or stdin input)
```

### SEE ALSO

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)

