## n8n config context use

Select the context used by subsequent commands

### Synopsis

Select the context used by subsequent commands.

NAME must already exist (see 'n8n config context list'). The choice takes
effect immediately for later runs. Create a new context with
'n8n auth login --context NAME'.

```
n8n config context use NAME [flags]
```

### Examples

```
  n8n config context use production
```

### Options

```
  -h, --help   help for use
```

### SEE ALSO

* [n8n config context](n8n_config_context.md)	 - Manage saved instance contexts

