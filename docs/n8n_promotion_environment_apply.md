## n8n promotion environment apply

Apply an environment's Git branch into this instance

### Synopsis

Check the full package at the branch tip of an instance-scope apply
environment (n8n 2.46 and later) and import it into the instance,
overwriting to match. Before 2.46 use 'n8n promotion apply' with a
connection. Start with 'n8n promotion environment list --direction
apply' and connect it first with 'n8n promotion environment connect'.
Personal projects are ignored. An applied run rewrites every team
project from the branch and cannot be undone: workflows, folders,
credentials, data tables, variables, and tags are created, updated,
or removed to match, workflows and folders move within their project,
folders take the branch name, and credential gaps become stubs.
Existing variable values are kept. A workflow or folder the branch and
the instance hold in different projects is refused with 409 before any
write. Requires gitConnection:pull.

Use --expected-from FILE|- with changes JSON or a blocked apply result,
or supply --expected-environment-id, --expected-branch, and
--expected-commit together (full lowercase 40- or 64-digit hex SHA). A
null commit cannot be used. --expected-from passes through the
identifier the instance returned: environmentId.
Expected source is optional; supply it to detect changes after review.
Exit codes: 0 applied, 3 blocked, 4 source-changed, 1 other error. Only
applied writes content. Resolve blocked bindings before apply-continue;
review source-changed results again. JSON prints the full status result;
text shows preflight details or counts and warnings. Hints go to stderr.
Read request JSON with --input FILE|- (maximum 1 MiB). Input and document flags are
mutually exclusive. Interactive runs ask for confirmation; non-interactive
runs and stdin input require --yes. JSON returns the server result; text
shows counts and the commit after success.

--idempotency-key KEY makes a retry after a timeout or lost response safe:
the same request with the same key returns the first result instead of
running again (auto generates a key and prints it on stderr).

```
n8n promotion environment apply <environment-id> [flags]
```

### Examples

```
  n8n promotion environment apply ENVIRONMENT_ID
  n8n promotion environment apply ENVIRONMENT_ID --input request.json --yes --output json
```

### Options

```
      --context string                   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --expected-branch string           reviewed branch name (default: omitted; requires the expected ID and commit)
      --expected-commit string           reviewed lowercase full 40- or 64-digit hex commit SHA (default: omitted; requires the expected ID and branch)
      --expected-environment-id string   reviewed environment ID, 1 to 36 characters (default: omitted; requires expected branch and commit)
      --expected-from string             changes or blocked-apply JSON file, or - for stdin (default: explicit expected-source flags; stdin requires --yes)
  -h, --help                             help for apply
      --idempotency-key string           retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)
      --input string                     request JSON file path or - for stdin, maximum 1 MiB (default: use document flags; stdin requires --yes)
      --output string                    output format: text or json (default: text; JSON is the full server result) (default "text")
      --url string                       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes                              confirm content changes without prompting (default: prompt; required for non-interactive or stdin input)
```

### SEE ALSO

* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)

