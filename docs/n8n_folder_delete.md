## n8n folder delete

Delete a folder and move or archive what it holds

### Synopsis

Delete one folder. What happens to its contents depends on the transfer target.

With --transfer-to-folder-id, the workflows and sub-folders are moved into that
folder first and nothing is archived or lost. Without it, the workflows are
moved to the project root and archived, and every folder underneath is deleted
with it. Run 'n8n folder get' first to see how much that is; the totals it
reports are recursive.

Interactive runs ask for confirmation. Non-interactive runs require --yes.
Requires folder:delete.

```
n8n folder delete <project-id> <folder-id> [flags]
```

### Examples

```
  n8n folder delete PROJECT_ID FOLDER_ID
  n8n folder delete PROJECT_ID FOLDER_ID --transfer-to-folder-id OTHER_FOLDER_ID --yes
  n8n folder delete PROJECT_ID FOLDER_ID --yes --output json
```

### Options

```
      --context string                 saved context to use (default: the current context; see 'n8n config context list')
  -h, --help                           help for delete
      --output string                  output format: text or json (acknowledgement contains the project, folder and transfer target) (default "text")
      --transfer-to-folder-id string   move the workflows and sub-folders into this folder first (default: archive the workflows at the project root and delete the child folders)
      --url string                     instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes                            confirm the deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n folder](n8n_folder.md)	 - Manage the folders of a project

