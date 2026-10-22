## n8n node-policy

Manage node type availability policies

### Synopsis

Read and replace the policies that decide which node types may be used, at
instance scope and per project. Preview API: n8n may change it without notice.

A scope has a default action and one or more attached policy documents, each an
ordered rule list; the first matching rule wins and the default applies to the
rest. 'get' reports the composed view: the rules of every attached document in
evaluation order. 'set' replaces only the scope's own document.

Start with 'instance get'. Writes are full replacements, so the workflow is get,
edit, set: start from 'get --output json', change the rules, and pass the whole
document back. The version field carries that round trip and a stale value is
rejected with 409, which means the policy or one of its attachments changed
first. Once a scope has more than its own document attached, a get-edit-set
round trip copies the rules of the shared documents into the scope's own
document; check the scope's attachments before editing one that has them.

Requires nodeTypePolicy:manage, n8n 2.40.0 or later, and the licensed
type-availability-policies module. Instances without it answer 404 or 503.

```
n8n node-policy [flags]
```

### Examples

```
  n8n node-policy instance get
  n8n node-policy instance get --output json > policy.json
  n8n node-policy instance set --input policy.json
  n8n node-policy project get L5fcpzBVjoU2PKFP
```

### Options

```
  -h, --help   help for node-policy
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n node-policy attachment](n8n_node-policy_attachment.md)	 - Manage attached node type policy documents
* [n8n node-policy document](n8n_node-policy_document.md)	 - Manage reusable node type policy documents
* [n8n node-policy instance](n8n_node-policy_instance.md)	 - Manage the instance node type policy
* [n8n node-policy project](n8n_node-policy_project.md)	 - Manage a project's node type policy

