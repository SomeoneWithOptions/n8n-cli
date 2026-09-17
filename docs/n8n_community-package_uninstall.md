## n8n community-package uninstall

Uninstall a community package

### Synopsis

Uninstall one community node package from the selected n8n instance.

This removes the package's code and nodes from the instance. It does not delete
workflows, credentials or execution data, but workflows that reference removed
nodes may stop loading or running. The API returns no package document.

Interactive runs ask before uninstalling. Non-interactive runs fail unless --yes
is passed. Requires API-key authentication and communityPackage:uninstall.
Next step: run 'n8n community-package list' and inspect affected workflows.

```
n8n community-package uninstall <name> [flags]
```

### Examples

```
  n8n community-package uninstall n8n-nodes-example
  n8n community-package uninstall n8n-nodes-example --yes
  n8n community-package uninstall '@scope/n8n-nodes-example' --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for uninstall
      --output string    output format: text or json (default text; json is stable for scripting) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes              confirm the instance-changing action without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n community-package](n8n_community-package.md)	 - Manage community node packages

