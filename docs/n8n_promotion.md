## n8n promotion

Manage promotion providers, connections, and apply/promote sync

### Synopsis

Manage the upstream-only Promotions generation of Git sync.
Start with 'provider list' to find a provider, then 'connection list'
to find a connection. Create a provider first, then a connection on
it, then a config per direction under 'config', clone each checkout
under 'checkout', link team projects under 'project', then 'promote'
local projects to the remote or 'apply' the remote into the instance.

'promote' commits and pushes every team project to the remote;
'apply' resets the apply checkout and imports with overwrite behavior,
creating, updating, or removing content to match. Both mutate versioned
content and ask for confirmation. Config and connection removal can
remove local checkouts. This is the upstream Promotions generation: an
instance that serves GitConnections instead answers 404 or 503, which
reads as the availability diagnostic.

```
n8n promotion [flags]
```

### Examples

```
  n8n promotion provider list
  n8n promotion connection list
  n8n promotion provider create --input provider.json
  n8n promotion connection create --input connection.json
  n8n promotion config apply set CONNECTION_ID --input apply.json
  n8n promotion checkout clone CONNECTION_ID apply
  n8n promotion project add CONNECTION_ID PROJECT_ID
  n8n promotion promote CONNECTION_ID --commit-message "sync" --yes
  n8n promotion apply CONNECTION_ID --yes
```

### Options

```
  -h, --help   help for promotion
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n promotion apply](n8n_promotion_apply.md)	 - Apply the Git remote into this instance
* [n8n promotion checkout](n8n_promotion_checkout.md)	 - Manage promotion checkouts
* [n8n promotion config](n8n_promotion_config.md)	 - Manage promotion direction configs
* [n8n promotion connection](n8n_promotion_connection.md)	 - Manage promotion connections
* [n8n promotion project](n8n_promotion_project.md)	 - Manage projects linked to a promotion connection
* [n8n promotion promote](n8n_promotion_promote.md)	 - Promote team projects to the Git remote
* [n8n promotion provider](n8n_promotion_provider.md)	 - Manage promotion providers

