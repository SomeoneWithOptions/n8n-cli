## n8n config context rename

Rename a saved context without changing its credentials

### Synopsis

Rename a saved context locally.

OLD must exist and NEW must be unused; an existing same-name rename is a no-op.
Names use 1–64 ASCII letters, digits, '-', '_' or '.', with no leading dot.
URL, authentication, storage and credential identity are preserved. If OLD is
current, the selection follows NEW. This CLI-side metadata change needs no
network or credential-store access and never changes remote resources.

No confirmation is needed. Success prints a message to stderr, leaving stdout
empty. External scripts, --context and N8N_CONTEXT still refer to literal names:
update N8N_CONTEXT after renaming its selected context, or unset it. Update
references to OLD yourself; no alias is created. Avoid writing shared config
with older binaries that reuse names as credential references.
Next step: 'n8n config context list --output json' to inspect saved state.

```
n8n config context rename OLD NEW [flags]
```

### Examples

```
  n8n config context rename default work
  n8n config context use work
```

### Options

```
  -h, --help   help for rename
```

### SEE ALSO

* [n8n config context](n8n_config_context.md)	 - Manage saved instance contexts

