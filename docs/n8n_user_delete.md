## n8n user delete

Permanently delete a user

### Synopsis

Permanently delete one user using the exact ID or email accepted by the API.
By default n8n also deletes the user's workflows and credentials. Pass
--transfer-to-project with a project ID (sent as the API's transferId) to move
those workflows and credentials to that project instead; find IDs with
'n8n project list'. Deletion cannot be undone and the instance owner cannot be
deleted.

Interactive runs ask for confirmation naming what happens to the user's
workflows and credentials. Non-interactive runs require --yes.
Requires user:delete.

```
n8n user delete <user-id-or-email> [flags]
```

### Examples

```
  n8n user delete USER_ID --transfer-to-project PROJECT_ID
  n8n user delete person@example.com --yes --output json
```

### Options

```
      --context string               saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help                         help for delete
      --output string                output format: text or json (acknowledgement contains deleted user identifier and any transfer project ID) (default "text")
      --transfer-to-project string   project ID that receives the user's workflows and credentials (default: none, they are deleted with the user)
      --url string                   instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes                          confirm permanent user deletion, and deletion of their workflows and credentials unless transferred, without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n user](n8n_user.md)	 - Manage instance users and global roles

