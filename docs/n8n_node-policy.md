## n8n node-policy

Manage node type availability policies

### Synopsis

Read and replace the policies that decide which node types may be used, at
instance scope and per project. A policy is a default action plus an ordered
rule list; the first matching rule wins and the default applies to the rest.

Every action is a full replacement, so the workflow is get, edit, set: start
from 'get --output json', change the rules, and pass the whole document back.
The version field carries that round trip and a stale value is rejected with
409, which means someone else changed the policy first.

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
* [n8n node-policy instance](n8n_node-policy_instance.md)	 - Manage the instance node type policy
* [n8n node-policy project](n8n_node-policy_project.md)	 - Manage a project's node type policy

