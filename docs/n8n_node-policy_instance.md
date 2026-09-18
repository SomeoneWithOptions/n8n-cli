## n8n node-policy instance

Manage the instance node type policy

### Synopsis

Read or fully replace the instance-scope node type policy. Instance scope is the
only scope that accepts the delegate action, which hands a decision down to the
project policy instead of settling it.

An instance that was never configured reports no rules, an allow default, and
version 0; writing to it creates the policy document.

```
n8n node-policy instance [flags]
```

### Examples

```
  n8n node-policy instance get
  n8n node-policy instance get --output json > policy.json
  n8n node-policy instance set --input policy.json
```

### Options

```
  -h, --help   help for instance
```

### SEE ALSO

* [n8n node-policy](n8n_node-policy.md)	 - Manage node type availability policies
* [n8n node-policy instance get](n8n_node-policy_instance_get.md)	 - Get the instance node type policy
* [n8n node-policy instance set](n8n_node-policy_instance_set.md)	 - Fully replace the instance node type policy

