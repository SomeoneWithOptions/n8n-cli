## n8n credential-policy attachment set

Replace credential type policy attachments

### Synopsis

Fully replace all attachments using JSON {"attachments":[{"policyId":"POLICY_ID","priority":0,"isFloor":false}],"version":1}. Each policyId and (isFloor,priority) pair must be unique. Use [] to detach every document. Get SCOPE_ID from 'instance get' or 'project get PROJECT_ID' -> scopeId; null means write that scope first. Requires instance-level access even for projects. Delegate documents cannot attach to projects. Use the last-read scope version: stale writes return 409. Replacing attachments changes policy evaluation and bumps the scope version. Interactive runs confirm; non-interactive runs and --input - require --yes. Returns attachments and the new version; read-only rules in GET input are dropped with a stderr diagnostic. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy attachment set <scope-id> [flags]
```

### Examples

```
  n8n credential-policy attachment set SCOPE_ID --input policy.json
  n8n credential-policy attachment set SCOPE_ID --input - --yes < policy.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for set
      --input string     JSON file path, or - for stdin (required; maximum 1 MiB; stdin requires --yes)
      --output string    output format: text or json (default: text; JSON contains the full response) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm mutation without prompting (default: false; required for non-interactive replacement/deletion and --input -)
```

### SEE ALSO

* [n8n credential-policy attachment](n8n_credential-policy_attachment.md)	 - Manage attached credential type policy documents

