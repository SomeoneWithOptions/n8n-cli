## n8n folder get

Show one folder with its totals

### Synopsis

Show one folder: its name, its parent folder, and how many sub-folders and
workflows it holds. Both totals are recursive, so they count everything in the
subtree, not only the direct children the list command reports.

Read those totals before deleting a folder: they are what the deletion moves,
archives or removes. Find folder IDs with 'n8n folder list'. Requires
folder:read.

```
n8n folder get <project-id> <folder-id> [flags]
```

### Examples

```
  n8n folder get PROJECT_ID FOLDER_ID
  n8n folder get PROJECT_ID FOLDER_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the folder object with its totals) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n folder](n8n_folder.md)	 - Manage the folders of a project

