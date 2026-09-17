## n8n config context

Manage saved instance contexts

### Synopsis

Manage saved instance contexts.

A context is one instance URL, one authentication type and a reference to the
credential stored for it. Contexts are kept apart so a credential for one
instance is never sent to another. List with 'list', switch with 'use NAME',
remove with 'delete NAME'. Create or repair one with 'n8n auth login'.

```
n8n config context [flags]
```

### Examples

```
  n8n config context list
  n8n config context use production
  n8n config context delete old-staging --yes
```

### Options

```
  -h, --help   help for context
```

### SEE ALSO

* [n8n config](n8n_config.md)	 - Inspect and edit stored CLI configuration
* [n8n config context delete](n8n_config_context_delete.md)	 - Delete a context and its stored credential
* [n8n config context list](n8n_config_context_list.md)	 - List saved contexts
* [n8n config context use](n8n_config_context_use.md)	 - Select the context used by subsequent commands

