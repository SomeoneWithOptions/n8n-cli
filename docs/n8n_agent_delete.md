## n8n agent delete

Permanently delete an Agent

### Synopsis

Permanently delete one Agent and its associated resources. It does not need
to be archived first: a published Agent is deleted too and its live channels
stop. This cannot be undone. Interactive runs ask for confirmation;
non-interactive runs require --yes. Requires agent:delete and n8n 2.49 or
later.

```
n8n agent delete <agent-id> [flags]
```

### Examples

```
  n8n agent delete AGENT_ID
  n8n agent delete AGENT_ID --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (JSON is the deleted Agent summary) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm permanent deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n agent](n8n_agent.md)	 - Manage Agents

