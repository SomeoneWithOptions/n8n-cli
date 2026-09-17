## n8n folder

Manage the folders of a project

### Synopsis

Manage folders inside one project. Every command takes the project ID first,
because folders live in a project and their IDs are only unique within one.

Start with 'list' to find folder IDs, then create, inspect, rename, move, or
delete a folder. Project IDs come from 'n8n project list'; 'create' also accepts
the literal 'personal' for the calling user's own personal project, which the
other commands do not. Deleting a folder moves or archives what it holds, so it
always requires confirmation.

```
n8n folder [flags]
```

### Examples

```
  n8n folder list PROJECT_ID
  n8n folder create PROJECT_ID Invoices
  n8n folder get PROJECT_ID FOLDER_ID
  n8n folder update PROJECT_ID FOLDER_ID --name "Paid invoices"
  n8n folder delete PROJECT_ID FOLDER_ID --transfer-to-folder-id OTHER_FOLDER_ID --yes
```

### Options

```
  -h, --help   help for folder
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n folder create](n8n_folder_create.md)	 - Create a folder in a project
* [n8n folder delete](n8n_folder_delete.md)	 - Delete a folder and move or archive what it holds
* [n8n folder get](n8n_folder_get.md)	 - Show one folder with its totals
* [n8n folder list](n8n_folder_list.md)	 - List the folders of a project
* [n8n folder update](n8n_folder_update.md)	 - Rename a folder or move it under another folder

