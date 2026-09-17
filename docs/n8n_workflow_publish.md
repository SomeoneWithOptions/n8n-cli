## n8n workflow publish

Publish a workflow so its triggers run

### Synopsis

Publish one workflow, which n8n v1 called activating it. The published version
goes live: schedule triggers start running on their schedule and webhook URLs
start accepting requests, in production.

Without --version-id the latest saved version is published; pass a version ID
from 'n8n workflow history' to publish an older one instead. --name and
--description label the published version and do not rename the workflow.

A workflow with no trigger node cannot be published, and a publication blocked
by an open workflow review or by a webhook path already in use answers 409 with
the reason. Reverse it with 'n8n workflow unpublish'. Requires
workflow:activate.

```
n8n workflow publish <workflow-id> [flags]
```

### Examples

```
  n8n workflow publish WORKFLOW_ID
  n8n workflow publish WORKFLOW_ID --version-id VERSION_ID
  n8n workflow publish WORKFLOW_ID --name "Release 2026-09-17" --description "adds the retry branch"
  n8n workflow publish WORKFLOW_ID --output json
```

### Options

```
      --context string       saved context to use (default: the current context; see 'n8n config context list')
      --description string   description for the published version (default: none)
  -h, --help                 help for publish
      --name string          label for the published version; does not rename the workflow (default: the version's existing name)
      --output string        output format: text or json (JSON is the published workflow) (default "text")
      --url string           instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --version-id string    version to publish, from 'n8n workflow history' (default: the latest saved version)
```

### SEE ALSO

* [n8n workflow](n8n_workflow.md)	 - Manage workflows, their versions and their tags

