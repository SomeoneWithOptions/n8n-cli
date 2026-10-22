## n8n community-package install

Install a community package

### Synopsis

Install an npm community node package on the selected n8n instance.

The package name must have the form n8n-nodes-<name> or the scoped form
@<scope>/n8n-nodes-<name>. --version takes a semver version or an npm dist-tag
such as latest or beta; omit it to let npm select the version. Whether
unverified packages may be installed is an instance setting: when it is off,
only versions verified by n8n install. Installed package code runs inside n8n.

Interactive runs ask before installing. Non-interactive runs fail unless --yes
is passed. Requires API-key authentication and communityPackage:install.
Next step: 'n8n community-package list'.

```
n8n community-package install <name> [flags]
```

### Examples

```
  n8n community-package install n8n-nodes-example
  n8n community-package install n8n-nodes-example --version 1.2.3 --yes
  n8n community-package install '@scope/n8n-nodes-example' --version beta --yes --output json
```

### Options

```
      --context string   saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
  -h, --help             help for install
      --output string    output format: text or json (default text; json is stable for scripting) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --version string   npm semver version or dist-tag, e.g. 1.2.3 or beta (default: let n8n select the version)
      --yes              confirm the instance-changing action without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n community-package](n8n_community-package.md)	 - Manage community node packages

