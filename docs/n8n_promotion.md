## n8n promotion

Manage promotion providers, environments, connections, and apply/promote sync

### Synopsis

Manage Promotions Git sync and selected project workflows.

n8n 2.46 and later: start with 'provider list' to find a provider,
then 'environment list' to find an environment. An environment is one
remote for one direction: create one per direction on a provider with
'environment create', prepare its checkout with 'environment connect',
and link team projects to a projects-scope environment under
'environment project'. 'promote', 'apply', and the project selection
commands still address connections.

Older instances serve connections instead: 'connection list' finds a
connection; create a provider first, then a connection on it, then a
config per direction under 'config', clone each checkout under
'checkout', link team projects to a projects-scope connection under
'project', then 'promote' the instance to the remote or 'apply' the
remote into the instance. For selected workflows, start with project
changes, then project apply or project promote. Use apply-continue
after resolving a blocked apply.

'promote' and 'apply' run on the instance connection. 'promote'
commits and pushes every team project to the remote; 'apply' checks the
package and imports with overwrite behavior, creating, updating, or
removing content to match, or reports why it is blocked and writes
nothing. Both mutate versioned content and ask for confirmation.
Config, connection, and environment removal can remove local
checkouts. This is the upstream Promotions generation: an instance
that serves GitConnections instead answers 404 or 503, which reads as
the availability diagnostic.

```
n8n promotion [flags]
```

### Examples

```
  n8n promotion provider list
  n8n promotion environment list
  n8n promotion environment connect ENVIRONMENT_ID
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
* [n8n promotion apply-continue](n8n_promotion_apply-continue.md)	 - Continue a reviewed connection apply after binding setup
* [n8n promotion checkout](n8n_promotion_checkout.md)	 - Manage promotion checkouts
* [n8n promotion config](n8n_promotion_config.md)	 - Manage promotion direction configs
* [n8n promotion connection](n8n_promotion_connection.md)	 - Manage promotion connections
* [n8n promotion environment](n8n_promotion_environment.md)	 - Manage promotion environments (n8n 2.46 and later)
* [n8n promotion project](n8n_promotion_project.md)	 - Manage projects linked to a promotion connection
* [n8n promotion promote](n8n_promotion_promote.md)	 - Promote team projects to the Git remote
* [n8n promotion provider](n8n_promotion_provider.md)	 - Manage promotion providers

