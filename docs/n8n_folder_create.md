## n8n folder create

Create a folder in a project

### Synopsis

Create one folder in one project. Without --parent-folder-id the folder is
created at the project root; with it, the folder is created inside that folder.

This command also accepts the literal project ID 'personal', which creates the
folder in the calling user's own personal project; list, get, update and delete
need the real project ID instead. Quote names containing spaces. Requires
folder:create.

```
n8n folder create <project-id> <name> [flags]
```

### Examples

```
  n8n folder create PROJECT_ID Invoices
  n8n folder create PROJECT_ID "Paid invoices" --parent-folder-id FOLDER_ID
  n8n folder create personal Invoices --output json
```

### Options

```
      --context string            saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help                      help for create
      --output string             output format: text or json (JSON is the created folder) (default "text")
      --parent-folder-id string   create the folder inside this folder (default: the project root)
      --url string                instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n folder](n8n_folder.md)	 - Manage the folders of a project

