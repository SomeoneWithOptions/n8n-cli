## n8n community-package install

Install a community package

### Synopsis

Install an npm community node package on the selected n8n instance.

The package name must start with n8n-nodes-. Omit --version to let npm select
the version. Verification against n8n's vetted package list is enabled by
default; --verify=false explicitly permits unverified code when the instance
allows it. Installed package code runs inside n8n.

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
  n8n community-package install n8n-nodes-example --verify=false --yes --output json
```

### Options

```
      --context string   saved context to use (default: the current context; see 'n8n config context list')
  -h, --help             help for install
      --output string    output format: text or json (default text; json is stable for scripting) (default "text")
      --url string       instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --verify           verify against n8n's vetted package list (default true; --verify=false permits unverified code when allowed) (default true)
      --version string   npm package version, e.g. 1.2.3 (default: let n8n select the version)
      --yes              confirm the instance-changing action without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n community-package](n8n_community-package.md)	 - Manage community node packages

