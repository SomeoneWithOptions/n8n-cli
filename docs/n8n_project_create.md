## n8n project create

Create a project

### Synopsis

Create one project with the given name. Name is the only writable field; the ID
and type are assigned by the instance.

The API contract declares no response body for this call, so an instance that
returns none is reported as created without an ID; run 'n8n project list' to
recover it. Quote names containing spaces. Add members afterwards with
'n8n project user add'. Requires project:create and a licensed instance.

```
n8n project create <name> [flags]
```

### Examples

```
  n8n project create "Billing automation"
  n8n project create Staging --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for create
      --output string    output format: text or json (JSON is the project object) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n project](n8n_project.md)	 - Manage projects and their members

