## n8n folder update

Rename a folder or move it under another folder

### Synopsis

Update one folder. This is a partial update: --name renames the folder,
--parent-folder-id moves it (with everything inside it) under another folder,
and passing both does both. At least one of them is required; anything left out
stays as it is.

--parent-folder-id 0 moves the folder back to the project root. Otherwise the
target folder must be in the same project. Names are at most 128 characters.
Requires folder:update.

```
n8n folder update <project-id> <folder-id> [flags]
```

### Examples

```
  n8n folder update PROJECT_ID FOLDER_ID --name "Paid invoices"
  n8n folder update PROJECT_ID FOLDER_ID --parent-folder-id OTHER_FOLDER_ID
  n8n folder update PROJECT_ID FOLDER_ID --parent-folder-id 0
  n8n folder update PROJECT_ID FOLDER_ID --name Archive --parent-folder-id OTHER_FOLDER_ID --output json
```

### Options

```
      --context string            saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help                      help for update
      --name string               new folder name (required unless --parent-folder-id is given)
      --output string             output format: text or json (JSON is the updated folder) (default "text")
      --parent-folder-id string   move the folder inside this folder, or 0 for the project root (required unless --name is given)
      --url string                instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n folder](n8n_folder.md)	 - Manage the folders of a project

