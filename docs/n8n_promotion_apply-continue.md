## n8n promotion apply-continue

Continue a reviewed connection apply after binding setup

### Synopsis

Continue a full instance-connection apply after resolving its preflight items.
Start with 'n8n promotion apply CONNECTION_ID --output json'. The apply
checkout must be cloned. Every team project is synchronized: workflows,
folders, credentials, tables, variables, and tags can change or be removed.
Requires gitConnection:pull and the same permissions as the initial apply.
Use --expected-from FILE|- with changes JSON or a blocked apply result,
or supply all three --expected-config-id, --expected-branch, --expected-commit
flags (full lowercase 40- or 64-digit hex SHA). A null commit cannot be used.
Expected source is required; use the blocked result's configId and git.
The server keeps no session and provides no exactly-once guarantee.
--confirm-destructive-changes permits removal or retyping of data-table
columns and loss of their values; rows and other columns remain.
It also permits deleting team projects with everything in them.
This covers every such change at continue time, including changes absent
from the reviewed result.
Exit codes: 0 applied, 3 blocked, 4 source-changed, 1 other error. Only
applied writes content. Resolve blocked bindings before apply-continue;
review source-changed results again. JSON prints the full status result;
text shows preflight details or counts and warnings. Hints go to stderr.
Read request JSON with --input FILE|- (maximum 1 MiB). Input and document flags are
mutually exclusive. Interactive runs ask for confirmation; non-interactive
runs and stdin input require --yes. JSON returns the server result; text
shows counts and the commit after success.

```
n8n promotion apply-continue <connection-id> [flags]
```

### Examples

```
  n8n promotion apply-continue CONNECTION_ID --expected-from blocked.json
  n8n promotion apply-continue CONNECTION_ID --input request.json --yes --output json
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
      --yes                           confirm content changes without prompting (default: prompt; required for non-interactive or stdin input)
```

### SEE ALSO

* [n8n promotion](n8n_promotion.md)	 - Manage promotion providers, connections, and apply/promote sync

