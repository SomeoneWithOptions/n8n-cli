## n8n node-policy project

Manage a project's node type policy

### Synopsis

Read or fully replace one project's own node type policy. The reported policy is
the project's own document, not the result of combining it with the instance
policy, so a node type can still be blocked by instance scope.

Project scope accepts allow and deny only; delegate is instance-scope. A project
that was never configured reports no rules, an allow default, and version 0.

```
n8n node-policy project [flags]
```

### Examples

```
  n8n node-policy project get L5fcpzBVjoU2PKFP
  n8n node-policy project get L5fcpzBVjoU2PKFP --output json > project-policy.json
  n8n node-policy project set L5fcpzBVjoU2PKFP --input project-policy.json
```

### Options

```
  -h, --help   help for project
```

### SEE ALSO

* [n8n node-policy](n8n_node-policy.md)	 - Manage node type availability policies
* [n8n node-policy project get](n8n_node-policy_project_get.md)	 - Get a project's node type policy
* [n8n node-policy project set](n8n_node-policy_project_set.md)	 - Fully replace a project's node type policy

