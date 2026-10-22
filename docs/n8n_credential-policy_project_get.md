## n8n credential-policy project get

Get the project credential type policy

### Synopsis

Read the composed project credential type policy, including every attached document in evaluation order. This does not combine project and instance evaluation. scopeId identifies attachments and is null until the scope is first written. Find project IDs with 'n8n project list'. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy project get <project-id> [flags]
```

### Examples

```
  n8n credential-policy project get PROJECT_ID
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (default: text; JSON contains the full response) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n credential-policy project](n8n_credential-policy_project.md)	 - Manage project credential type policies

