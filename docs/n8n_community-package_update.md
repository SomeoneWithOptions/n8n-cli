## n8n community-package update

Update an installed community package

### Synopsis

Update one installed community node package on the selected n8n instance.

--version takes a semver version or an npm dist-tag such as latest or beta.
Omit it to send no body and update to the latest version n8n allows. Whether
unverified versions may be installed is an instance setting: when it is off,
only versions verified by n8n install. Updating code can change node behavior
in existing workflows, though it does not delete those workflows.

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
      --version string   npm semver version or dist-tag, e.g. 1.2.3 or beta (default: let n8n select the version)
      --yes              confirm the instance-changing action without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n community-package](n8n_community-package.md)	 - Manage community node packages

