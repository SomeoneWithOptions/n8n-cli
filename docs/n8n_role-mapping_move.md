## n8n role-mapping move

Move a rule in its evaluation order

### Synopsis

Move one rule to a new position in the evaluation order of its own type.
--target-index is the desired 0-based position among rules of that type; an
index beyond the last position moves the rule to the end.

Reordering changes which rule wins for a claim that several rules match, so it
changes the roles users receive at their next sign-in. The rules that shift
around the moved one keep a contiguous sequence. Run
'n8n role-mapping list --type TYPE' before and after to confirm the order.
Requires roleMappingRule:update.

```
n8n role-mapping move <rule-id> [flags]
```

### Examples

```
  n8n role-mapping move RULE_ID --target-index 0
  n8n role-mapping move RULE_ID --target-index 99 --output json
```

### Options

```
      --context string     saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help               help for move
      --output string      output format: text or json (JSON is the moved rule returned by the API) (default "text")
      --target-index int   0-based position within the rule's own type (required; a larger index moves the rule to the end)
      --url string         instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
```

### SEE ALSO

* [n8n role-mapping](n8n_role-mapping.md)	 - Manage identity-provider role-mapping rules

