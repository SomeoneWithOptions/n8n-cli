## n8n promotion config apply set

Replace the apply direction config

### Synopsis

Replace the apply direction config of one promotion connection from
strict JSON supplied by --input. The settings object is required:
{"settings":{"schemaVersion":1,"branchName":"main"}} with an
optional name. This creates the config on first write.

Interactive runs ask for confirmation and non-interactive runs require
--yes. Requires gitConnection:update. Clone the checkout next with
'n8n promotion checkout clone CONNECTION_ID apply'.

```
n8n promotion config apply set <connection-id> [flags]
```

### Examples

```
  n8n promotion config apply set CONNECTION_ID --input apply.json
  printf '%s\n' '{"settings":{"schemaVersion":1,"branchName":"main"}}' | n8n promotion config apply set CONNECTION_ID --input - --yes
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for set
      --input string     direction config JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the stored config returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm the config replacement without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n promotion config apply](n8n_promotion_config_apply.md)	 - Manage the apply direction config

