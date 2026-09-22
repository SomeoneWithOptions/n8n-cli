## n8n role-mapping update

Update a role-mapping rule

### Synopsis

Update one rule from strict JSON supplied by --input. This is a PATCH: only the
fields present are changed, and at least one of expression, role, or projectIds
is required.

A rule's type cannot be changed after creation, and its position is changed with
'n8n role-mapping move', so neither type nor order is accepted here. Pass
"projectIds": [] to clear a project rule's projects. Run
'n8n role-mapping list --output json' to read the current rule first.
Requires roleMappingRule:update.

```
n8n role-mapping update <rule-id> [flags]
```

### Examples

```
  n8n role-mapping update RULE_ID --input rule-update.json
  printf '%s\n' '{"role":"global:member"}' | n8n role-mapping update RULE_ID --input -
  printf '%s\n' '{"projectIds":[]}' | n8n role-mapping update RULE_ID --input - --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for update
      --input string     role-mapping rule JSON file path, or - for stdin (required; maximum 1 MiB)
      --output string    output format: text or json (JSON is the rule returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n role-mapping](n8n_role-mapping.md)	 - Manage identity-provider role-mapping rules

