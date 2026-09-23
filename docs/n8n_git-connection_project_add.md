## n8n git-connection project add

Add a project to a Git connection

### Synopsis

Add one team project to one Git connection so push and pull include
it. A project can belong to only one connection: adding a project
that is already linked elsewhere answers 409. The repository must be
cloned first. Requires gitConnection:manageProjects.

```
n8n git-connection project add <connection-id> <project-id> [flags]
```

### Examples

```
  n8n git-connection project add CONNECTION_ID PROJECT_ID
  n8n git-connection project add CONNECTION_ID PROJECT_ID --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for add
      --output string    output format: text or json (JSON is the project link returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n git-connection project](n8n_git-connection_project.md)	 - Manage projects linked to a Git connection

