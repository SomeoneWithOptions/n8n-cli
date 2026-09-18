## n8n promotion project add

Add a project to a promotion connection

### Synopsis

Add one team project to one promotion connection so promote and apply
include it. A project can belong to only one connection: adding a
project that is already linked elsewhere answers 409. The checkout
must be cloned first. Requires gitConnection:manageProjects.

```
n8n promotion project add <connection-id> <project-id> [flags]
```

### Examples

```
  n8n promotion project add CONNECTION_ID PROJECT_ID
  n8n promotion project add CONNECTION_ID PROJECT_ID --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for add
      --output string    output format: text or json (JSON is the project link returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n promotion project](n8n_promotion_project.md)	 - Manage projects linked to a promotion connection

