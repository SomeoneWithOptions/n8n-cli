## n8n node-policy project get

Get a project's node type policy

### Synopsis

Get one project's own composed node type policy: its default action and the rules
of its policy document, in evaluation order. This is not the effective decision
for the project, because the instance policy is applied on top at evaluation.

A project that was never configured reports no rules, defaultAction allow, and
version 0. Use --output json as the input for 'set'. Requires
nodeTypePolicy:manage. Find project IDs with 'n8n project list'.

```
n8n node-policy project get <project-id> [flags]
```

### Examples

```
  n8n node-policy project get L5fcpzBVjoU2PKFP
  n8n node-policy project get L5fcpzBVjoU2PKFP --output json > project-policy.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the full document 'set' expects) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n node-policy project](n8n_node-policy_project.md)	 - Manage a project's node type policy

