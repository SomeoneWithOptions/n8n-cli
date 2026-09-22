## n8n node-policy instance get

Get the instance node type policy

### Synopsis

Get the composed instance node type policy: its default action and the rules of
every attached policy document, in evaluation order. Use --output json as the
starting point for a get-edit-set workflow; the version it reports is what 'set'
must send back.

A scope that was never configured reports no rules, defaultAction allow, and
version 0. Requires nodeTypePolicy:manage.

```
n8n node-policy instance get [flags]
```

### Examples

```
  n8n node-policy instance get
  n8n node-policy instance get --output json
  n8n node-policy instance get --output json > policy.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the full document 'set' expects) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n node-policy instance](n8n_node-policy_instance.md)	 - Manage the instance node type policy

