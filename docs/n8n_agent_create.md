## n8n agent create

Create an Agent

### Synopsis

Create an Agent in your personal project or a team project you can access.
--name is enough for a draft; pass config (name, model, instructions) and
custom tools in --input JSON. An empty model creates an unconfigured draft.
Flags override the --input values. Each tools entry maps a tool name to
{"code": ..., "descriptor": {"name": ...}}.

--input accepts 'n8n agent get --output json' to copy an Agent: its
read-only fields (id, project, isArchived, versions, timestamps,
configHash, activeVersion) are dropped and reported on stderr; the copy is
an unpublished draft. --input - requires --yes. Requires agent:create and
n8n 2.49 or later.

--idempotency-key KEY makes a retry after a timeout or lost response safe:
the same request with the same key returns the first result instead of
running again (auto generates a key and prints it on stderr).

```
n8n agent create [flags]
```

### Examples

```
  n8n agent create --name Support --project-id PROJECT_ID
  n8n agent create --input agent.json --name "Support (copy)"
  n8n agent get AGENT_ID --output json | n8n agent create --input - --project-id PROJECT_ID --yes
```

### Options

```
      --available-in-mcp         make the Agent available to MCP clients; overrides availableInMCP in --input (default: the --input value, else the server default)
      --context string           saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help                     help for create
      --idempotency-key string   retry key sent as the Idempotency-Key header: 1 to 128 visible ASCII characters, or auto for a random UUID printed on stderr; rerun the same command with the same key to get the first result instead of a second run (default: none, no idempotency)
      --input string             create document JSON file path, or - for stdin, maximum 1 MiB (default: none; --name is then required)
      --name string              Agent name; overrides name in --input (default: the --input value)
      --output string            output format: text or json (JSON is the created Agent) (default "text")
      --project-id string        project to create the Agent in; overrides projectId in --input (default: the --input value, else your personal project)
      --url string               instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes                      confirm reading the document from stdin (required with --input -)
```

### SEE ALSO

* [n8n agent](n8n_agent.md)	 - Manage Agents

