## n8n ldap sync run

Run LDAP synchronization

### Synopsis

Start one LDAP synchronization and print its completed history record. --type dry
scans LDAP and reports proposed changes without persisting user changes. --type live
creates and updates users and may disable users missing from LDAP; live mode requires
interactive confirmation or --yes. Requires ldap:sync, configured LDAP settings,
and the LDAP license.

```
n8n ldap sync run [flags]
```

### Examples

```
  n8n ldap sync run --type dry
  n8n ldap sync run --type dry --output json
  n8n ldap sync run --type live
  n8n ldap sync run --type live --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for run
      --output string    output format: text or json (result is one synchronization history record) (default "text")
      --type string      synchronization mode: dry previews without user changes; live applies user changes (required)
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm live synchronization without prompting, including possible user disablement (required for non-interactive live runs)
```

### SEE ALSO

* [n8n ldap sync](n8n_ldap_sync.md)	 - Inspect or run LDAP synchronization

