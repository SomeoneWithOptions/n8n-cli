## n8n config context delete

Delete a context and its stored credential

### Synopsis

Delete a context and its stored credential.

This removes the entry from config.json and the credential it references from
the credential store. It changes nothing on the n8n instance: an API key deleted
here stays valid until it is revoked in n8n. Use --yes for scripts; without
a terminal the command fails unless --yes is given.

```
n8n config context delete NAME [flags]
```

### Examples

```
  n8n config context delete old-staging
  n8n config context delete old-staging --yes
```

### Options

```
  -h, --help   help for delete
      --yes    delete without asking (required in non-interactive use)
```

### SEE ALSO

* [n8n config context](n8n_config_context.md)	 - Manage saved instance contexts

