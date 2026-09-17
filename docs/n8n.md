## n8n

Command-line client for the n8n API

### Synopsis

Command-line client for the n8n public API.

Start here: log in once, check the credential, then see what the instance and
that credential can do before running resource commands.
  n8n auth login --url https://n8n.example.com
  n8n auth status --check
  n8n discover

Configuration precedence for non-secrets is: flag, environment variable
(N8N_URL and friends), selected context, default. Secrets are never flags:
they come from a no-echo prompt, --stdin, the environment, or the OS
credential store.

Commands write machine-readable results to stdout and diagnostics, prompts,
and confirmations to stderr. Use --output json for scripting; use --help on
any group or action (for example 'n8n auth --help') for its workflow and flags.

```
n8n [flags]
```

### Examples

```
  n8n auth login --url https://n8n.example.com
  n8n auth status --check
  n8n discover --resource workflow
  n8n audit generate
  n8n config context list
  n8n --help
  n8n auth login --help
```

### Options

```
  -h, --help      help for n8n
      --version   version for n8n
```

### SEE ALSO

* [n8n audit](n8n_audit.md)	 - Generate security audits of an n8n instance
* [n8n auth](n8n_auth.md)	 - Log in to an n8n instance and inspect stored credentials
* [n8n community-package](n8n_community-package.md)	 - Manage community node packages
* [n8n completion](n8n_completion.md)	 - Generate a shell completion script
* [n8n config](n8n_config.md)	 - Inspect and edit stored CLI configuration
* [n8n credential](n8n_credential.md)	 - Manage node credentials without exposing secrets
* [n8n data-table](n8n_data-table.md)	 - Manage data tables, their rows and their columns
* [n8n discover](n8n_discover.md)	 - List the API capabilities available to the current credential
* [n8n folder](n8n_folder.md)	 - Manage the folders of a project
* [n8n project](n8n_project.md)	 - Manage projects and their members
* [n8n role](n8n_role.md)	 - Manage global and project roles
* [n8n tag](n8n_tag.md)	 - Manage workflow tags
* [n8n user](n8n_user.md)	 - Manage instance users and global roles
* [n8n variable](n8n_variable.md)	 - Manage instance and project variables
* [n8n version](n8n_version.md)	 - Print build information

