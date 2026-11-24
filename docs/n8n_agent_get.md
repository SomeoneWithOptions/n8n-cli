## n8n agent get

Get one Agent with its configuration

### Synopsis

Get one Agent by ID, as listed by 'n8n agent list'. Text shows the project,
publication, configuration hash, custom tool names, and channel
integrations; JSON is the full Agent, including config and tools. The JSON
can seed 'n8n agent create --input': read-only fields are dropped there.
Read-only; requires agent:read and n8n 2.49 or later.

```
n8n agent get <agent-id> [flags]
```

### Examples

```
  n8n agent get AGENT_ID
  n8n agent get AGENT_ID --output json > agent.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for get
      --output string    output format: text or json (JSON is the full Agent, including config and tools) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n agent](n8n_agent.md)	 - Manage Agents

