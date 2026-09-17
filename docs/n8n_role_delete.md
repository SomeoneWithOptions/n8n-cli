## n8n role delete

Permanently delete a custom role

### Synopsis

Permanently delete one custom role. System roles cannot be deleted. A role with
assigned users is refused unless --reassign-role names another compatible role;
n8n moves those users before deleting the role. Users themselves are not deleted.

Deletion cannot be undone. Interactive runs ask for confirmation; non-interactive
runs require --yes. Requires role:manage or role:manageProject as appropriate.

```
n8n role delete <role-slug> [flags]
```

### Examples

```
  n8n role delete global:custom
  n8n role delete global:custom --reassign-role global:member --yes
  n8n role delete project:custom --yes --output json
```

### Options

```
      --context string         saved context to use (default: the current context; see 'n8n config context list')
  -h, --help                   help for delete
      --output string          output format: text or json (JSON is the deleted role returned by the API) (default "text")
      --reassign-role string   role slug receiving assigned users before deletion (default: do not reassign; deletion fails when users remain)
      --url string             instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes                    confirm permanent role deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n role](n8n_role.md)	 - Manage global and project roles

