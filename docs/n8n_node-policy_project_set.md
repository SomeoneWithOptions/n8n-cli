## n8n node-policy project set

Fully replace a project's node type policy

### Synopsis

Replace one project's default action and every rule of its policy document from
strict JSON. This is a PUT, not a patch: rules, defaultAction, and version are
all required, and any rule left out is deleted. Start from
'n8n node-policy project get <project-id> --output json'.

Project scope accepts allow and deny only; delegate belongs to the instance
policy and is rejected before the request is sent. Selectors are
{"kind":"name"} or {"kind":"package"}, rule ids must be unique, and rules are
evaluated in list order.

version must equal the version last read; a stale value, or a policy document
shared with another scope, is rejected with 409 and nothing changes. Blocking a
node type stops workflows in the project that use it, so interactive runs ask
for confirmation and non-interactive runs require --yes. Requires
nodeTypePolicy:manage.

```
n8n node-policy project set <project-id> [flags]
```

### Examples

```
  n8n node-policy project get L5fcpzBVjoU2PKFP --output json > project-policy.json
  n8n node-policy project set L5fcpzBVjoU2PKFP --input project-policy.json
  n8n node-policy project set L5fcpzBVjoU2PKFP --input - --yes < project-policy.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for set
      --input string     full node type policy JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the stored policy plus shadowed-rule warnings) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm the full replacement without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n node-policy project](n8n_node-policy_project.md)	 - Manage a project's node type policy

