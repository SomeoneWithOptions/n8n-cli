## n8n variable

Manage instance and project variables

### Synopsis

Manage variables available to n8n expressions. Start with 'list' to inspect
visible global and project-scoped variables, then create, update or delete them.

List output contains variable values because retrieval is the requested operation.
Create and update never repeat a submitted value in success or error diagnostics.
Keys are unique within their scope. Omitting --project-id on a write selects global
scope; project variables need the relevant project access on the instance.

```
n8n variable [flags]
```

### Examples

```
  n8n variable list
  n8n variable create API_HOST --value https://api.example.com
  n8n variable update VARIABLE_ID --key API_HOST --value https://api.internal
  n8n variable delete VARIABLE_ID --yes
```

### Options

```
  -h, --help   help for variable
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n variable create](n8n_variable_create.md)	 - Create a variable
* [n8n variable delete](n8n_variable_delete.md)	 - Permanently delete a variable
* [n8n variable list](n8n_variable_list.md)	 - List variables and their values
* [n8n variable update](n8n_variable_update.md)	 - Fully replace a variable

