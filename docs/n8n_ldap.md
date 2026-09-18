## n8n ldap

Manage licensed LDAP settings and synchronization

### Synopsis

Read and fully replace the instance LDAP configuration, inspect synchronization
history, and start dry or live synchronization runs. Start with 'get --output
json', keep the password placeholder unchanged unless replacing or clearing the
secret, then pass the complete edited document to 'set'. Protect any file where
you enter a replacement bind password.

Disabling LDAP login deletes stored LDAP identities and stops synchronization. A
live sync creates, updates, and may disable users. Both actions require explicit
confirmation. Configuration requires ldap:manage; sync actions require ldap:sync;
all commands require the LDAP license.

```
n8n ldap [flags]
```

### Examples

```
  n8n ldap get
  umask 077 && n8n ldap get --output json > ldap.json
  n8n ldap set --input ldap.json
  n8n ldap sync history
  n8n ldap sync run --type dry
```

### Options

```
  -h, --help   help for ldap
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n ldap get](n8n_ldap_get.md)	 - Get LDAP configuration
* [n8n ldap set](n8n_ldap_set.md)	 - Fully replace LDAP configuration
* [n8n ldap sync](n8n_ldap_sync.md)	 - Inspect or run LDAP synchronization

