## n8n credential-policy document set

Replace credential type policy documents

### Synopsis

Fully replace a document's rules using JSON {"rules":[],"version":1}. Read the document first. This changes every attached scope and bumps each scope's version; omitted rules are deleted. The server rejects delegate when any attached scope is a project. Stale versions return 409: read again before retrying. Returns {policy,warnings}. Interactive runs confirm; non-interactive runs and --input - require --yes. Selectors accept name or package. Credential policies also accept extends, matching the named type and every type extending it. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy document set <policy-id> [flags]
```

### Examples

```
  n8n credential-policy document set POLICY_ID --input policy.json
  n8n credential-policy document set POLICY_ID --input - --yes < policy.json
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

* [n8n credential-policy document](n8n_credential-policy_document.md)	 - Manage reusable credential type policy documents

