## n8n ldap sync

Inspect or run LDAP synchronization

### Synopsis

List cursor-paginated LDAP synchronization history or start one run. Use
'history' first to inspect recent outcomes. A dry run scans and reports proposed
changes without changing users. A live run creates, updates, and may disable users,
so it requires confirmation or --yes. Requires ldap:sync and the LDAP license.

```
n8n ldap sync [flags]
```

### Examples

```
  n8n ldap sync history
  n8n ldap sync history --all --output json
  n8n ldap sync run --type dry
  n8n ldap sync run --type live --yes
```

### Options

```
  -h, --help   help for sync
```

### SEE ALSO

* [n8n ldap](n8n_ldap.md)	 - Manage licensed LDAP settings and synchronization
* [n8n ldap sync history](n8n_ldap_sync_history.md)	 - List LDAP synchronization history
* [n8n ldap sync run](n8n_ldap_sync_run.md)	 - Run LDAP synchronization

