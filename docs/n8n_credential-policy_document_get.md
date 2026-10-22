## n8n credential-policy document get

Get credential type policy documents

### Synopsis

Read a reusable document's ID, kind, ordered rules, version, updater and timestamps. Find POLICY_ID with 'document list'. Edit rules and use the last-read version with 'document set'; read-only fields are dropped with a stderr diagnostic. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy document get <policy-id> [flags]
```

### Examples

```
  n8n credential-policy document get POLICY_ID
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (default: text; JSON contains the full response) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n credential-policy document](n8n_credential-policy_document.md)	 - Manage reusable credential type policy documents

