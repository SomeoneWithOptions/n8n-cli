## n8n community-package update

Update an installed community package

### Synopsis

Update one installed community node package on the selected n8n instance.

Omit --version to update to the version selected by n8n. Verification against
n8n's vetted package list is enabled by default; --verify=false explicitly
permits unverified code when the instance allows it. Updating code can change
node behavior in existing workflows, though it does not delete those workflows.

Interactive runs ask before updating. Non-interactive runs fail unless --yes is
passed. Requires API-key authentication and communityPackage:update.
Next step: run 'n8n community-package list' and test affected workflows.

```
n8n community-package update <name> [flags]
```

### Examples

```
  n8n community-package update n8n-nodes-example
  n8n community-package update n8n-nodes-example --version 2.0.0 --yes
  n8n community-package update '@scope/n8n-nodes-example' --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for update
      --output string    output format: text or json (default text; json is stable for scripting) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --verify           verify against n8n's vetted package list (default true; --verify=false permits unverified code when allowed) (default true)
      --version string   npm package version, e.g. 1.2.3 (default: let n8n select the version)
      --yes              confirm the instance-changing action without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n community-package](n8n_community-package.md)	 - Manage community node packages

