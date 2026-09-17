## n8n community-package

Manage community node packages

### Synopsis

Manage npm community node packages installed on an n8n instance.

Start with 'n8n community-package list'. Install, update and uninstall change
code loaded by the instance and therefore require confirmation, or --yes in a
non-interactive run. Workflows are not deleted, but changing or removing a
package can break workflows that use its nodes.

This API group accepts API-key authentication only. Bearer tokens and browser
cookies cannot be used. Typical workflow: login with --type api-key, list
installed packages, then install, update or uninstall one package.

```
n8n community-package [flags]
```

### Examples

```
  n8n community-package list
  n8n community-package install n8n-nodes-example
  n8n community-package update n8n-nodes-example --yes
  n8n community-package uninstall n8n-nodes-example --yes
```

### Options

```
  -h, --help   help for community-package
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n community-package install](n8n_community-package_install.md)	 - Install a community package
* [n8n community-package list](n8n_community-package_list.md)	 - List installed community packages
* [n8n community-package uninstall](n8n_community-package_uninstall.md)	 - Uninstall a community package
* [n8n community-package update](n8n_community-package_update.md)	 - Update an installed community package

