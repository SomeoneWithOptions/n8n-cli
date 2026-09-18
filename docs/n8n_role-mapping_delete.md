## n8n role-mapping delete

Permanently delete a role-mapping rule

### Synopsis

Permanently delete one role-mapping rule. Users already signed in keep the role
they were granted, but the rule stops applying at their next sign-in, so people
who depended on it can lose access. Deletion cannot be undone.

The remaining rules of the same type close the gap, so their order values stay
a contiguous sequence starting at 0. Interactive runs ask for confirmation;
non-interactive runs require --yes. Run 'n8n role-mapping list' first to find
the ID. Requires roleMappingRule:delete.

```
n8n role-mapping delete <rule-id> [flags]
```

### Examples

```
  n8n role-mapping delete RULE_ID
  n8n role-mapping delete RULE_ID --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for delete
      --output string    output format: text or json (JSON is the deleted rule returned by the API) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm permanent deletion without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n role-mapping](n8n_role-mapping.md)	 - Manage identity-provider role-mapping rules

