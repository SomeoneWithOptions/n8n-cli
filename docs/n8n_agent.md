## n8n agent

Manage Agents

### Synopsis

Manage the Agents of an n8n instance (n8n 2.49 and later).

An Agent lives in a project and holds a configuration (name, model,
instructions), custom tools, and once published, channel integrations.
Start with 'list' to find the agent ID, then get, create, or delete.
Instances before n8n 2.49 answer 404. Moving Agents between instances
belongs to 'n8n package export' and 'n8n package import'.

```
n8n agent [flags]
```

### Examples

```
  n8n agent list --project-id PROJECT_ID
  n8n agent get AGENT_ID --output json > agent.json
  n8n agent create --name Support --project-id PROJECT_ID
  n8n agent delete AGENT_ID --yes
```

### Options

```
  -h, --help   help for agent
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n agent create](n8n_agent_create.md)	 - Create an Agent
* [n8n agent delete](n8n_agent_delete.md)	 - Permanently delete an Agent
* [n8n agent get](n8n_agent_get.md)	 - Get one Agent with its configuration
* [n8n agent list](n8n_agent_list.md)	 - List Agents

