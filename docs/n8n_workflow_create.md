## n8n workflow create

Create a workflow from a definition or as an empty one

### Synopsis

Create one workflow. With --input the definition is read from a JSON file or
from stdin ('--input -'); without it, --name creates an empty workflow with no
nodes, ready to be edited in the UI or filled in by a later update.

The input may be a document produced by 'n8n workflow get --output json': the
read-only fields (id, versionId, active, timestamps, tags, sharing) are dropped
before sending, because the API answers a read-only field with 400, and the
dropped names are reported on stderr. --name, --project-id and
--parent-folder-id override what the document says.

A created workflow is never published: publish it with 'n8n workflow publish'.
Requires workflow:create.

```
n8n workflow create [flags]
```

### Examples

```
  n8n workflow create --name "Invoice sync"
  n8n workflow create --input workflow.json
  n8n workflow create --input - < workflow.json
  n8n workflow create --input workflow.json --name "Invoice sync copy" --project-id PROJECT_ID
  n8n workflow create --name Scratch --output json
```

### Options

```
      --context string            saved context to use (default: the current context; see 'n8n config context list')
  -h, --help                      help for create
      --input string              workflow definition as JSON: a file path, or - for stdin (default: an empty workflow built from --name)
      --name string               workflow name; overrides the name in --input (required when --input is absent)
      --output string             output format: text or json (JSON is the created workflow) (default "text")
      --parent-folder-id string   folder to create the workflow in, from 'n8n folder list PROJECT_ID' (default: the project root)
      --project-id string         project to create the workflow in, from 'n8n project list' (default: the caller's personal project)
      --url string                instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

