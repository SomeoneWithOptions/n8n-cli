## n8n variable delete

Permanently delete a variable

### Synopsis

Permanently delete one variable by ID. Workflows are not deleted, but expressions
that rely on the variable may stop resolving or behave differently. The deletion
cannot be undone.

Interactive runs ask for confirmation. Non-interactive runs require --yes. Use
'n8n variable list' first to find the ID. Requires variable:delete.

```
n8n variable delete <variable-id> [flags]
```

### Examples

```
  n8n variable delete VARIABLE_ID
  n8n variable delete VARIABLE_ID --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (acknowledgement contains only the deleted variable ID) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm permanent deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n variable](n8n_variable.md)	 - Manage instance and project variables

