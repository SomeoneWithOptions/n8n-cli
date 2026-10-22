## n8n node-policy document delete

Delete node type policy documents

### Synopsis

Permanently delete an unattached document found with 'document list'. Attached documents return 409: inspect 'attachment get' and use 'attachment set' to detach first. Interactive runs confirm; non-interactive runs require --yes. Success returns the deleted ID; deletion cannot be undone. Preview API: breaking changes may occur. Requires nodeTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n node-policy document delete <policy-id> [flags]
```

### Examples

```
  n8n node-policy document delete POLICY_ID
  n8n node-policy document delete POLICY_ID --yes
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (default: text; JSON contains the full response) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm mutation without prompting (default: false; required for non-interactive replacement/deletion and --input -)
```

### SEE ALSO

* [n8n node-policy document](n8n_node-policy_document.md)	 - Manage reusable node type policy documents

