## n8n role-mapping

Manage identity-provider role-mapping rules

### Synopsis

Manage the rules that turn identity-provider claims into n8n roles at login.
Start with 'list' to read the current rules, then create, update, move, or
delete them.

Rules are evaluated in order within their own type: instance rules grant a
global role, project rules grant a project role on the projects they name, and
each type has its own sequence starting at 0. Filter 'list' by --type to read a
single evaluation order. Rules only take effect for users signing in through
SAML or OIDC; see 'n8n saml get' and 'n8n oidc get' for the provider itself.

```
n8n role-mapping [flags]
```

### Examples

```
  n8n role-mapping list --type instance
  n8n role-mapping create --input rule.json
  n8n role-mapping update RULE_ID --input rule-update.json
  n8n role-mapping move RULE_ID --target-index 0
  n8n role-mapping delete RULE_ID --yes
```

### Options

```
  -h, --help   help for role-mapping
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n role-mapping create](n8n_role-mapping_create.md)	 - Create a role-mapping rule
* [n8n role-mapping delete](n8n_role-mapping_delete.md)	 - Permanently delete a role-mapping rule
* [n8n role-mapping list](n8n_role-mapping_list.md)	 - List role-mapping rules
* [n8n role-mapping move](n8n_role-mapping_move.md)	 - Move a rule in its evaluation order
* [n8n role-mapping update](n8n_role-mapping_update.md)	 - Update a role-mapping rule

