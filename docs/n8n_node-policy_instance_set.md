## n8n node-policy instance set

Fully replace the instance node type policy

### Synopsis

Replace the instance default action and every rule of its policy document from
strict JSON. This is a PUT, not a patch: rules, defaultAction, and version are
all required, and any rule left out is deleted. Start from
'n8n node-policy instance get --output json'.

Actions are allow, deny, or delegate. Selectors are {"kind":"name"} for one node
type or {"kind":"package"} for every node type in a package. Rule ids must be
unique and rules are evaluated in list order.

version must equal the version last read; a stale value is rejected with 409 and
nothing changes. Blocking a node type stops workflows that use it, so
interactive runs ask for confirmation and non-interactive runs require --yes.
scopeId and warnings from a previous response are accepted and ignored.
Requires nodeTypePolicy:manage.

```
n8n node-policy instance set [flags]
```

### Examples

```
  n8n node-policy instance get --output json > policy.json
  n8n node-policy instance set --input policy.json
  n8n node-policy instance set --input policy.json --yes --output json
  n8n node-policy instance set --input - --yes < policy.json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for set
      --input string     full node type policy JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the stored policy plus shadowed-rule warnings) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm the full replacement without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n node-policy instance](n8n_node-policy_instance.md)	 - Manage the instance node type policy

