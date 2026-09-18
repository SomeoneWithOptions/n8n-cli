## n8n promotion config promote set

Replace the promote direction config

### Synopsis

Replace the promote direction config of one promotion connection from
strict JSON supplied by --input. The settings object is required:
{"settings":{"schemaVersion":1,"baseBranchName":"main",
"createBranchOnPromotion":false}} with an optional name. This creates
the config on first write.

Interactive runs ask for confirmation and non-interactive runs require
--yes. Requires gitConnection:update. Clone the checkout next with
'n8n promotion checkout clone CONNECTION_ID promote'.

```
n8n promotion config promote set <connection-id> [flags]
```

### Examples

```
  n8n promotion config promote set CONNECTION_ID --input promote.json
  n8n promotion config promote set CONNECTION_ID --input promote.json --yes --output json
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

* [n8n promotion config promote](n8n_promotion_config_promote.md)	 - Manage the promote direction config

