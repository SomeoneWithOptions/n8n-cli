## n8n config

Inspect and edit stored CLI configuration

### Synopsis

Inspect and edit stored CLI configuration.

Configuration is contexts: one instance URL plus auth type plus credential
reference each. Start with 'n8n config context list', switch with
'n8n config context use NAME'.

```
n8n config [flags]
```

### Examples

```
  n8n config context list
  n8n config context use production
```

### Options

```
  -h, --help   help for config
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n config context](n8n_config_context.md)	 - Manage saved instance contexts

