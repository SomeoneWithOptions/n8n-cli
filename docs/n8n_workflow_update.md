## n8n workflow update

Replace the definition of a workflow

### Synopsis

Replace one workflow's definition with the document given by --input, a JSON
file or stdin ('--input -').

This is a full replacement, not a patch: nodes, connections, settings and
pinned data that the document leaves out are cleared on the server. Always
start from a fresh read:
  n8n workflow get ID --output json > workflow.json
  n8n workflow update ID --input workflow.json
Read-only fields in the document (id, versionId, active, timestamps, tags,
sharing) are dropped before sending and reported on stderr, because the API
answers a read-only field with 400.

A published workflow is republished with the new version unless --no-publish is
given. Republishing additionally needs the workflow:activate scope and the
project's workflow:publish permission; without them the new version is still
saved as a draft and the command fails with 403, leaving the published version
live. Requires workflow:update.

```
n8n workflow update <workflow-id> [flags]
```

### Examples

```
  n8n workflow get WORKFLOW_ID --output json > workflow.json
  n8n workflow update WORKFLOW_ID --input workflow.json
  n8n workflow update WORKFLOW_ID --input - < workflow.json
  n8n workflow update WORKFLOW_ID --input workflow.json --no-publish
  n8n workflow update WORKFLOW_ID --input workflow.json --name "Invoice sync v2"
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for update
      --input string     workflow definition as JSON: a file path, or - for stdin (required)
      --name string      workflow name; overrides the name in --input (default: the name in the document)
      --no-publish       save the change as a draft instead of republishing a published workflow (default: republish)
      --output string    output format: text or json (JSON is the updated workflow) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

