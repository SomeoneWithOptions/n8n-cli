## n8n credential-policy project set

Replace the project credential type policy

### Synopsis

Fully replace the project defaultAction and rules of its own document using rules, defaultAction, and version last read (0 before first write). Omitted rules are deleted. GET includes shared rules: remove those before using its response as input, or they will be copied into the own document. Selectors accept name, package, or extends; extends matches a named credential type and every type extending it. Project scope rejects delegate. Stale versions or shared project documents return 409. Blocking credential types can stop workflows using them. Interactive runs confirm; non-interactive runs and --input - require --yes. scopeId and warnings from GET are dropped and reported on stderr. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy project set <project-id> [flags]
```

### Examples

```
  n8n credential-policy project set PROJECT_ID --input policy.json
  n8n credential-policy project set PROJECT_ID --input - --yes < policy.json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for set
      --input string     JSON file path, or - for stdin (required; maximum 1 MiB; stdin requires --yes)
      --output string    output format: text or json (default: text; JSON contains the full response) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes              confirm mutation without prompting (default: false; required for non-interactive replacement/deletion and --input -)
```

### SEE ALSO

* [n8n credential-policy project](n8n_credential-policy_project.md)	 - Manage project credential type policies

