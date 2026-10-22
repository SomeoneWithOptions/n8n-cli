## n8n credential-policy document create

Create credential type policy documents

### Synopsis

Create an unattached reusable document from JSON {"rules":[]}. Rules are required; each rule has a unique id, action allow|deny|delegate, and selector {kind,value}. Creation returns {policy,warnings}; warnings do not fail the write. Attach the returned policy.id using 'attachment set'. --input - requires --yes. Selectors accept name or package. Credential policies also accept extends, matching the named type and every type extending it. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy document create [flags]
```

### Examples

```
  n8n credential-policy document create --input policy.json
  n8n credential-policy document create --input - --yes < policy.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for create
      --input string     JSON file path, or - for stdin (required; maximum 1 MiB; stdin requires --yes)
      --output string    output format: text or json (default: text; JSON contains the full response) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm mutation without prompting (default: false; required for non-interactive replacement/deletion and --input -)
```

### SEE ALSO

* [n8n credential-policy document](n8n_credential-policy_document.md)	 - Manage reusable credential type policy documents

