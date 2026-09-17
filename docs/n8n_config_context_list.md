## n8n config context list

List saved contexts

### Synopsis

List saved contexts.

Shows every saved context, which one is current (*), and its URL, auth
type, and storage. Prints a hint when none exists yet. Use --output json
for scripting.

```
n8n config context list [flags]
```

### Examples

```
  n8n config context list
  n8n config context list --output json
```

### Options

```
  -h, --help            help for list
      --output string   output format: text or json (default text; json is stable for scripting) (default "text")
```

### SEE ALSO

* [n8n config context](n8n_config_context.md)	 - Manage saved instance contexts

