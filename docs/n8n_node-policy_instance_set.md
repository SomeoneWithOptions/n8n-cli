## n8n node-policy instance set

Fully replace the instance node type policy

### Synopsis

Replace the instance default action and every rule of its own policy document
from strict JSON. This is a PUT, not a patch: rules, defaultAction, and version
are all required, and any rule left out is deleted. Start from
'n8n node-policy instance get --output json'.

'get' reports the rules of every attached document, but 'set' writes only the
instance's own document. When shared documents are attached, remove their
rules from the input or they are copied into the own document; the attached
documents themselves are not changed.

Actions are allow, deny, or delegate. Selectors are {"kind":"name"} for one node
type or {"kind":"package"} for every node type in a package. Rule ids must be
unique and rules are evaluated in list order.

version must equal the version last read; a stale value is rejected with 409 and
nothing changes. Blocking a node type stops workflows that use it, so
interactive runs ask for confirmation and non-interactive runs require --yes.
scopeId and warnings from a previous response are accepted and ignored.

The response lists warnings that do not fail the write: kind shadowed means an
earlier rule already decides the rule's node types, and kind notLoaded means the
rule selects node types this instance has not loaded. Preview API: n8n may
change it without notice. Requires nodeTypePolicy:manage.

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
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for set
      --input string     full node type policy JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the stored policy plus warnings, each with a kind) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm the full replacement without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n node-policy instance](n8n_node-policy_instance.md)	 - Manage the instance node type policy

